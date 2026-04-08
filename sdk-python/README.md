# kairos-sdk

Typed sync and async Python clients for the Kairos local runtime.

## Status

`kairos-sdk` lives in this repo and tracks the local Kairos API. Treat it as pre-1.0 while the main runtime is still stabilizing.

## Install

```bash
python -m pip install -e ./sdk-python
```

For development and tests:

```bash
python -m pip install -e ./sdk-python[dev]
```

## Run Tests

From the repo root:

```bash
make test-python
```

## Example

```python
from kairos_sdk import Client

client = Client(base_url="http://127.0.0.1:7777")

health = client.health()
print(health.status, health.version)
```
