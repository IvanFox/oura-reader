class OuraReaderError(Exception):
    """Base exception for oura-reader client."""


class AuthError(OuraReaderError):
    """Authentication failed."""


class SyncError(OuraReaderError):
    """Sync operation failed."""
