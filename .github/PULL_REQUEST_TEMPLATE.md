## Summary

What does this PR change, and why?

## Checklist

- [ ] `make check` passes (build + uncached tests + vet + gofmt)
- [ ] `make race` passes if this touches the agent loop, UI update paths, or transports
- [ ] Golden fixtures regenerated if rendering changed (`go test ./internal/ui -run TestGoldenRender -update`)
- [ ] `CHANGELOG.md` updated for user-visible changes
- [ ] Commit messages follow the repo convention (`feat:`, `fix:`, `docs:`, …)
