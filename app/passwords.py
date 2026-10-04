"""Cryptographically-secure password generation."""

import secrets
import string

MIN_LENGTH = 4
MAX_LENGTH = 128


def generate_password(length: int = 16, use_symbols: bool = False) -> str:
    """Generate a random password using the `secrets` module (not `random`).

    Alphanumeric by default; optionally adds a small, URL/shell-safe symbol set.
    Length is clamped to a sane range regardless of what's requested.
    """
    length = max(MIN_LENGTH, min(int(length), MAX_LENGTH))

    alphabet = string.ascii_letters + string.digits
    if use_symbols:
        alphabet += "!@#$%^&*()-_=+"

    return "".join(secrets.choice(alphabet) for _ in range(length))
