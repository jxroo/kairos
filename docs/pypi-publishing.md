# PyPI publishing for `kairos-sdk`

The release workflow (`.github/workflows/release.yml`, job `publish-pypi`)
uploads the Python SDK to PyPI on every `v*` tag push. It supports two
authentication paths. You only need to set up **one** of them.

## Path A: API token (recommended for the first release)

This is the simpler path and works without touching PyPI's trusted-publisher
configuration. Do this once.

1. Create a scoped PyPI API token:
   - Go to https://pypi.org/manage/account/token/
   - **Token name:** `kairos-sdk github release` (any name is fine)
   - **Scope:** `Project: kairos-sdk` after the first successful upload, or
     "Entire account (all projects)" for the very first upload (because the
     project does not exist yet). You can rotate this to a project-scoped
     token immediately after the first release.
   - Copy the token. It starts with `pypi-...` and is shown only once.

2. Add the token as a GitHub Actions secret:
   - Go to https://github.com/jxroo/kairos/settings/secrets/actions/new
   - **Name:** `PYPI_API_TOKEN` (exact casing)
   - **Value:** the `pypi-...` token you copied.
   - Save.

3. That's it. The next `v*` tag push will use the token automatically. The
   workflow step `Publish to PyPI (token)` runs whenever the secret is set;
   otherwise it falls through to path B.

## Path B: Trusted publisher (OIDC)

This path uses short-lived OIDC tokens with no long-lived secrets. It is more
secure long-term but requires one configuration step on PyPI **before the
project exists**.

1. Go to https://pypi.org/manage/account/publishing/
2. Scroll to **Add a new pending publisher** and fill in:
   - **PyPI Project Name:** `kairos-sdk`
   - **Owner:** `jxroo`
   - **Repository name:** `kairos`
   - **Workflow name:** `release.yml`
   - **Environment name:** *(leave empty — the workflow does not set one)*
3. Submit.

When the next `v*` tag push runs the release workflow, PyPI will see the
OIDC claims from the workflow and match them against the pending publisher
you configured. The first successful publish converts the pending publisher
into a regular trusted publisher.

**Do not set `PYPI_API_TOKEN`** if you want to use this path. When the token
secret is absent, the workflow's `Publish to PyPI (trusted publisher)` step
runs instead.

## Why both paths exist

The first v0.1.0 and v0.1.1 release runs tried trusted publishing and failed
with:

```
invalid-publisher: valid token, but no corresponding publisher
```

This error means no pending publisher was configured on PyPI at upload time.
Rather than requiring a manual retry after PyPI config, the workflow now
supports the token path as a reliable primary option and keeps the trusted
publisher path as a fallback for installations that prefer it.

## Verifying a publish

After the workflow finishes successfully:

```bash
pip install kairos-sdk==X.Y.Z  # exact version of the tag
python -c "from kairos_sdk import Client; Client()"
```

Or browse to https://pypi.org/project/kairos-sdk/ and check the version list.

## Rotating the token

If the API token is leaked, revoke it at https://pypi.org/manage/account/token/
and create a new one with the same name. Update the `PYPI_API_TOKEN` secret
in GitHub. No workflow changes needed.
