# Security policy

`gb` handles a secret that links mirror commits to your real ones, and it writes to a GitHub repo with your credentials. A bug in either is a security bug.

## Supported versions

Only the latest release gets fixes.

## Report a vulnerability

Report privately through [GitHub private vulnerability reporting](https://github.com/sorfeb/greenbridge/security/advisories/new). Don't open a public issue.

Include the `gb version` output, your OS, and the steps to reproduce. You get a reply within 7 days.

In scope, for example:

- anything that lets a mirror commit reveal more than its timestamp and opaque ID
- recovering the secret or a source commit from mirror commits
- `gb` writing outside its config directory or the mirror repo

## Verify a release

Every release archive is listed in `checksums.txt`, and each release has a GitHub build provenance attestation. Check a download before you run it:

```sh
gh attestation verify gb_<version>_linux_amd64.tar.gz --repo sorfeb/greenbridge
```

A pass means the file was built by this repo's release workflow from a tagged commit, not uploaded by hand.
