#!/usr/bin/env bash
# Import the release signing subkey into a throwaway keyring and unlock it in
# gpg-agent, so GoReleaser can sign without prompting. Same approach as the
# KrakenKey package signing.
#
# Needs: GPG_PRIVATE_KEY (subkey-only export), GPG_PASSPHRASE,
#        GPG_SIGNING_SUBKEY (fingerprint), GNUPGHOME (empty directory).
set -euo pipefail

die() { echo "import-signing-key: $*" >&2; exit 1; }
[[ -n "${GPG_PRIVATE_KEY:-}" ]] || die "GPG_PRIVATE_KEY is empty"
[[ -n "${GPG_SIGNING_SUBKEY:-}" ]] || die "GPG_SIGNING_SUBKEY is empty"
[[ -n "${GNUPGHOME:-}" ]] || die "GNUPGHOME is not set"

mkdir -p "$GNUPGHOME"
chmod 700 "$GNUPGHOME"
echo "allow-preset-passphrase" >"$GNUPGHOME/gpg-agent.conf"
gpg --batch --quiet --import <<<"$GPG_PRIVATE_KEY"

# The CI copy must hold the signing subkey only, never a usable primary key.
if gpg --list-secret-keys --with-colons | awk -F: '/^sec:/ && $15 != "#" {found=1} END {exit !found}'; then
  die "the imported key includes the primary secret key; export the subkey only"
fi

KEYGRIP=$(gpg --list-secret-keys --with-colons --with-keygrip |
  awk -F: -v fpr="$GPG_SIGNING_SUBKEY" '/^ssb:/ {s=1; next} s && /^fpr:/ {match_=($10 == fpr)} s && match_ && /^grp:/ {print $10; exit}')
[[ -n "$KEYGRIP" ]] || die "signing subkey $GPG_SIGNING_SUBKEY is not in GPG_PRIVATE_KEY"

"$(gpgconf --list-dirs libexecdir)/gpg-preset-passphrase" --preset "$KEYGRIP" <<<"${GPG_PASSPHRASE:-}"

# Fail here, not halfway through a release, if the passphrase is wrong.
echo test | gpg --batch --pinentry-mode error --local-user "${GPG_SIGNING_SUBKEY}!" --detach-sign >/dev/null ||
  die "GPG_PASSPHRASE does not unlock signing subkey $GPG_SIGNING_SUBKEY"
echo "signing subkey $GPG_SIGNING_SUBKEY ready"
