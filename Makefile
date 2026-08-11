.PHONY: generate lint breaking clean

generate:
	buf generate

lint:
	buf lint

# Checks this branch against main. The proto is a published contract; this is
# the guard that keeps v1alpha honest once anything depends on it.
breaking:
	buf breaking --against '.git#branch=main'

clean:
	rm -rf gen
