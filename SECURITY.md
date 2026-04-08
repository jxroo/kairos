# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Kairos, please report it responsibly:

- **Email:** security@jxroo.dev
- **GitHub Security Advisories:** [Report a vulnerability](https://github.com/jxroo/kairos/security/advisories/new)

Do **not** open a public issue for security vulnerabilities.

## Scope

### In scope

- Kairos daemon (`kairos` binary)
- Kairos SDK and client libraries
- Installer script (`install.sh`) and Homebrew formula
- Official container images and distribution artifacts

### Out of scope

- Your LLM provider (Ollama, llama.cpp) — report issues to their respective projects
- Your operating system or hardware
- Third-party plugins or integrations not maintained by this project

## Response timeline

- **Acknowledgement:** within 48 hours of report
- **Triage and assessment:** within 7 days
- **Fix for critical vulnerabilities:** within 90 days
- **Fix for non-critical vulnerabilities:** best effort, typically within 120 days

## Disclosure

We follow a coordinated disclosure process:

1. Reporter submits the vulnerability privately.
2. We acknowledge and investigate.
3. We develop and test a fix.
4. We release the fix and publish an advisory.
5. Reporter is credited (unless they prefer to remain anonymous).

Please allow us reasonable time to address the issue before any public disclosure.

## Note

Kairos is designed for **local use on trusted networks**. It is not intended to be deployed as an internet-facing service. Running Kairos on an untrusted network or exposing it to the public internet is outside the supported threat model.
