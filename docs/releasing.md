# Releasing

A release is a version tag. `.github/workflows/release.yml` builds, on macOS:

- the `agent-desk` command for macOS and Linux, arm64 and amd64, as `agent-desk_<version>_<os>_<arch>.tar.gz` (`make release-cli`);
- the universal macOS app as `Agent-Desk_<version>_macos.zip` (`make desktop-universal`);
- `SHA256SUMS` for all of them (`make checksums`);

and creates a **draft** GitHub release with those files. Check it, then publish it.

```sh
git tag v0.1.0 && git push origin v0.1.0
```

Run by hand from the Actions tab, the workflow builds the same files as artifacts and makes no release. The same targets run locally; `VERSION=v0.1.0` sets the version without a tag.

## Signing and notarization

The app is signed with a Developer ID Application certificate and notarized when these repository secrets exist. Without them the build is unsigned and the run says so.

| Secret | Value |
| --- | --- |
| `APPLE_CERTIFICATE` | the certificate exported from Keychain Access as `.p12`, base64 encoded |
| `APPLE_CERTIFICATE_PASSWORD` | the `.p12` password |
| `APPLE_SIGNING_IDENTITY` | `Developer ID Application: Name (TEAMID)` |
| `APPLE_API_ISSUER` | App Store Connect API issuer id |
| `APPLE_API_KEY` | App Store Connect API key id |
| `APPLE_API_KEY_P8` | the key's `.p8` file, as text |

The signing identity, Team ID and name are public in every signed copy; the email on the certificate request is not.

## Homebrew

The formula installs the command from the release tarballs and depends on `agent-manager` and `tmux`; the cask installs the app (signed builds only). Templates live in `packaging/homebrew/`. After publishing a release:

```sh
scripts/homebrew.sh 0.1.0      # downloads SHA256SUMS from release v0.1.0
```

writes `agent-desk.rb` and `agent-desk-app.rb` to `packaging/homebrew/out/`. Copy them into the tap repository (`gsperry/homebrew-tap`, `Formula/` and `Casks/`). Then:

```sh
brew install gsperry/tap/agent-desk          # the command
brew install --cask gsperry/tap/agent-desk-app   # the app
```
