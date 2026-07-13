# Creating releases

Releases are fully automatic. You never build or upload anything by hand —
you just push a git tag, and GitHub does the rest.

## What happens when you push a version tag

1. GitHub Actions notices the tag (any tag starting with `v`).
2. It builds the frontend, then builds the `hdf` binary for:
   - macOS (Intel and Apple Silicon)
   - Linux (amd64 and arm64)
3. It creates a GitHub release for the tag and attaches:
   - one `.tar.gz` archive per platform (binary + README + docs)
   - a `checksums.txt` file so downloads can be verified
   - an automatic changelog built from the commit messages

You can watch it run under the repository's **Actions** tab ("Release"
workflow). It usually takes a few minutes.

## Making a full release

Pick the next version number (for example `v0.1.0`), then run:

```bash
git checkout main
git pull
git tag v0.1.0
git push origin v0.1.0
```

That's it. When the workflow finishes, the release appears on the
repository's **Releases** page.

## Making a pre-release

A pre-release is for versions you want people to try before they are final —
release candidates, betas, and so on. GitHub shows them with a "Pre-release"
label and does not mark them as the latest release.

To make one, add a suffix like `-rc.1` or `-beta.1` to the version:

```bash
git tag v0.1.0-rc.1
git push origin v0.1.0-rc.1
```

Any tag with a `-something` suffix is automatically published as a
pre-release. Plain tags (like `v0.1.0`) are always full releases. Nothing
else needs to change.

## Checking the version of a binary

Released binaries know their own version:

```bash
hdf --version
```

Binaries built by hand with `go build` say `dev` instead.

## Trying a release build without publishing anything

If you want to check that release builds still work (for example after
changing dependencies), run this locally:

```bash
goreleaser release --snapshot --clean
```

It builds everything into the `dist/` folder and publishes nothing.
(`dist/` is throwaway output; don't commit it.)

## Fixing a mistake

If you tagged the wrong commit or want to redo a release:

```bash
# Delete the tag locally and on GitHub.
git tag -d v0.1.0
git push --delete origin v0.1.0
```

Then delete the release from the **Releases** page on GitHub (deleting the
tag does not delete the release), fix what needed fixing, and tag again.

Never reuse a version number that people may have already downloaded —
when in doubt, just release the next number.
