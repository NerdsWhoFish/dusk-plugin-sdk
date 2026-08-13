// Package plugin runs a plugin against the host that started it.
//
// Binding the socket, requiring the host's token and shutting down politely
// are the same in every plugin, and were copied between them until this
// existed. Anything about how the host and a plugin meet belongs here, so
// changing that convention is one edit rather than one per plugin.
package plugin

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	duskv1alpha1 "github.com/NerdsWhoFish/dusk-plugin-sdk/gen/dusk/v1alpha1"
)

// SocketEnv is how the host names the socket to serve on.
const SocketEnv = "DUSK_PLUGIN_SOCKET"

// TokenEnv is the secret the host mints per start and presents on every call.
// Sockets share one directory and one user, so another plugin can dial this
// one; requiring the token keeps composition going through Dusk.
const TokenEnv = "DUSK_PLUGIN_TOKEN"

// TokenHeader is the metadata key carrying it.
const TokenHeader = "dusk-plugin-token"

// maxSocketPath is the portable floor for a unix socket address. The real
// limit is 104 bytes on macOS and BSD, 108 on Linux, and exceeding it fails as
// "invalid argument", which names neither the path nor the length.
const maxSocketPath = 104

// Options configures one plugin process. The zero value reads the host's
// environment, which is what a plugin started by Dusk wants.
type Options struct {
	// Socket defaults to $DUSK_PLUGIN_SOCKET. Set it to run by hand.
	Socket string

	// Token defaults to $DUSK_PLUGIN_TOKEN. Empty disables the check, which is
	// what makes grpcurl against a hand-started plugin possible.
	Token string

	// Version is reported in the start log, and is what the host compares an
	// available release against.
	Version string

	Log *slog.Logger
}

func (o Options) socket() string {
	if o.Socket != "" {
		return o.Socket
	}
	return os.Getenv(SocketEnv)
}

func (o Options) token() string {
	if o.Token != "" {
		return o.Token
	}
	return os.Getenv(TokenEnv)
}

func (o Options) log() *slog.Logger {
	if o.Log != nil {
		return o.Log
	}
	return slog.Default()
}

// Run serves a plugin until the host stops it. Starting with no socket is an
// error naming what sets it, because a plugin waiting forever on its own is
// indistinguishable from one that is working.
func Run(service duskv1alpha1.PluginServiceServer, opts Options) error {
	socket := opts.socket()
	if socket == "" {
		return fmt.Errorf("no socket to serve on: the host sets %s, or set Options.Socket to run this by hand", SocketEnv)
	}

	listener, err := Listen(socket)
	if err != nil {
		return err
	}

	server := grpc.NewServer(RequireToken(opts.token())...)
	duskv1alpha1.RegisterPluginServiceServer(server, service)

	// The host owns this process, so a signal is a normal shutdown rather than
	// a failure: stop serving, let in-flight calls finish, and leave.
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stopping)

	go func() {
		<-stopping
		server.GracefulStop()
	}()

	opts.log().Info("serving", "socket", socket, "version", opts.Version)
	if err := server.Serve(listener); err != nil {
		return fmt.Errorf("serve %q: %w", socket, err)
	}
	return nil
}

// Listen binds the socket, clearing a stale one first. A previous run killed
// hard leaves the file behind, and bind fails on it rather than replacing it.
func Listen(socket string) (net.Listener, error) {
	if len(socket) >= maxSocketPath {
		return nil, fmt.Errorf("socket path is %d bytes and the limit is %d: %q. Put the host's socket directory somewhere shorter",
			len(socket), maxSocketPath, socket)
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		return nil, fmt.Errorf("make the socket directory: %w", err)
	}
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("clear the stale socket %q: %w", socket, err)
	}

	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("listen on %q: %w", socket, err)
	}
	return listener, nil
}

// RequireToken refuses calls that do not present the host's token. An empty
// token disables the check, for running the plugin by hand against grpcurl.
func RequireToken(token string) []grpc.ServerOption {
	if token == "" {
		return nil
	}

	return []grpc.ServerOption{
		grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handle grpc.UnaryHandler) (any, error) {
			if err := check(ctx, token); err != nil {
				return nil, err
			}
			return handle(ctx, req)
		}),
		grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handle grpc.StreamHandler) error {
			if err := check(stream.Context(), token); err != nil {
				return err
			}
			return handle(srv, stream)
		}),
	}
}

func check(ctx context.Context, token string) error {
	presented := metadata.ValueFromIncomingContext(ctx, TokenHeader)
	for _, candidate := range presented {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) == 1 {
			return nil
		}
	}
	return status.Error(codes.Unauthenticated,
		"this plugin serves Dusk only. Compose through Dusk rather than calling another plugin directly")
}
