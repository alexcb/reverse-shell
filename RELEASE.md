Deploy notes:

```bash
export RELEASE_TAG=v0.0.1
git tag "$RELEASE_TAG"
git push origin "$RELEASE_TAG"

export GITHUB_TOKEN=...https://github.com/settings/tokens

earthly --build-arg RELEASE_TAG --secret GITHUB_TOKEN --push +release
```
