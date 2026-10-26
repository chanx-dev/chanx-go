MODULES := . layers layers/redis adapters/gin adapters/echo adapters/fiber examples docs

.PHONY: test lint docs docs-dev release release-dry

test:
	@for m in $(MODULES); do (cd $$m && go test -race ./...) || exit 1; done

lint:
	@for m in $(MODULES); do (cd $$m && golangci-lint run ./...) || exit 1; done

docs:
	@cd docs && pnpm install --frozen-lockfile && pnpm build

docs-dev:
	@cd docs && pnpm install && pnpm dev

# Each changed module bumped from the conventional commits since its last tag.
release:
	@scripts/release.sh

release-dry:
	@scripts/release.sh --dry-run
