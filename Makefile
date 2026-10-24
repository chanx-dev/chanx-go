MODULES := . layers layers/redis adapters/gin adapters/echo adapters/fiber examples

.PHONY: test lint release release-dry

test:
	@for m in $(MODULES); do (cd $$m && go test -race ./...) || exit 1; done

lint:
	@for m in $(MODULES); do (cd $$m && golangci-lint run ./...) || exit 1; done

# Version bumped from the conventional commits since the last tag.
release:
	@scripts/release.sh

release-dry:
	@scripts/release.sh --dry-run
