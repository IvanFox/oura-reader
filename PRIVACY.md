# Privacy Policy

oura-reader is a self-hosted, personal application. It is not a hosted service.

## Data collection and storage

- Data fetched from the Oura API is stored only on the user's own server in a local SQLite database.
- No data is sent to third parties.
- No analytics, telemetry, or tracking of any kind.

## Authentication and security

- OAuth tokens are encrypted at rest using AES-256-GCM before being stored in the database.
- API keys are hashed with SHA-256 and never stored in plain text.

## Data deletion

Users can delete all of their data at any time by running:

```
oura-reader user remove --name <username>
```

This removes the user account, all stored health data, and all OAuth tokens.

## Contact

For questions, open an issue on the [GitHub repository](https://github.com/ivan-lissitsnoi/oura-reader).
