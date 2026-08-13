package plugin_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	duskv1alpha1 "github.com/NerdsWhoFish/dusk-plugin-sdk/gen/dusk/v1alpha1"
	"github.com/NerdsWhoFish/dusk-plugin-sdk/plugin"
)

type stub struct {
	duskv1alpha1.UnimplementedPluginServiceServer
}

func (stub) Describe(context.Context, *duskv1alpha1.DescribeRequest) (*duskv1alpha1.DescribeResponse, error) {
	return &duskv1alpha1.DescribeResponse{PluginId: "stub"}, nil
}

// shortDir is not t.TempDir, because macOS puts that under a path long enough
// to blow the 104-byte socket limit before a test has added a filename.
func shortDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "dusk")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// serving starts a plugin on its own socket and returns a client for it.
func serving(t *testing.T, token string) duskv1alpha1.PluginServiceClient {
	t.Helper()

	socket := filepath.Join(shortDir(t), "p.sock")
	go func() {
		if err := plugin.Run(stub{}, plugin.Options{Socket: socket, Token: token}); err != nil {
			t.Errorf("run: %v", err)
		}
	}()

	conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return duskv1alpha1.NewPluginServiceClient(conn)
}

// describe retries while the process races to bind, so a slow start is not a
// failure the test reports as a refusal.
func describe(ctx context.Context, t *testing.T, client duskv1alpha1.PluginServiceClient) error {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := client.Describe(ctx, &duskv1alpha1.DescribeRequest{})
		if err == nil || status.Code(err) != codes.Unavailable || time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunServesOnTheSocketTheHostNamed(t *testing.T) {
	if err := describe(t.Context(), t, serving(t, "")); err != nil {
		t.Fatalf("describe: %v", err)
	}
}

// ADR-0042: every socket shares one directory and one user, so a plugin can
// dial another's. The token is what makes doing so useless, and keeps
// composition going through Dusk rather than between plugins.
func TestADR0042_APluginServesOnlyTheHostThatStartedIt(t *testing.T) {
	client := serving(t, "the-host-token")

	tests := []struct {
		name      string
		presented string
		allowed   bool
	}{
		{name: "the host's own token", presented: "the-host-token", allowed: true},
		{name: "another plugin dialing with nothing", presented: "", allowed: false},
		{name: "a guess", presented: "the-host-toke", allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			if test.presented != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, plugin.TokenHeader, test.presented)
			}

			err := describe(ctx, t, client)
			if test.allowed {
				if err != nil {
					t.Fatalf("expected the host to be served, got %v", err)
				}
				return
			}

			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("expected Unauthenticated, got %v", err)
			}
			if !strings.Contains(status.Convert(err).Message(), "Compose through Dusk") {
				t.Fatalf("the refusal should say what to do instead, got %q", status.Convert(err).Message())
			}
		})
	}
}

func TestRunWithNoSocketSaysWhatSetsIt(t *testing.T) {
	t.Setenv(plugin.SocketEnv, "")

	err := plugin.Run(stub{}, plugin.Options{})
	if err == nil {
		t.Fatal("expected a plugin with nowhere to serve to refuse to start")
	}
	if !strings.Contains(err.Error(), plugin.SocketEnv) {
		t.Fatalf("the error should name %s, got %q", plugin.SocketEnv, err)
	}
}

func TestListen(t *testing.T) {
	t.Run("clears a socket a hard kill left behind", func(t *testing.T) {
		socket := filepath.Join(shortDir(t), "stale.sock")
		if err := os.WriteFile(socket, nil, 0o600); err != nil {
			t.Fatalf("write the stale socket: %v", err)
		}

		listener, err := plugin.Listen(socket)
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		_ = listener.Close()
	})

	t.Run("refuses a path too long to bind", func(t *testing.T) {
		socket := filepath.Join(shortDir(t), strings.Repeat("a", 104)+".sock")

		_, err := plugin.Listen(socket)
		if err == nil {
			t.Fatal("expected an over-long socket path to be refused before bind turns it into invalid argument")
		}
		if !strings.Contains(err.Error(), "104") {
			t.Fatalf("the error should name the limit, got %q", err)
		}
	})
}

func TestRequireTokenIsOffWhenThereIsNoToken(t *testing.T) {
	if options := plugin.RequireToken(""); options != nil {
		t.Fatalf("an empty token should leave the server unguarded for grpcurl, got %d options", len(options))
	}
	if options := plugin.RequireToken("x"); len(options) == 0 {
		t.Fatal("a token should install the interceptors")
	}
}
