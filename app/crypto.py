"""Thin wrapper around msoffcrypto-tool for the web app's encrypt/decrypt flows.

Notes on limitations (see https://pypi.org/project/msoffcrypto-tool/):
- Decryption supports both legacy binary Office formats (.doc/.xls/.ppt) and
  modern OOXML formats (.docx/.xlsx/.pptx).
- Encryption is OOXML-only (.docx/.xlsx/.pptx) and is flagged upstream as
  experimental. There is no encryption path for the legacy binary formats.
"""

import msoffcrypto
from msoffcrypto.format.ooxml import OOXMLFile

# Modes -> which extensions are actually usable for that operation.
SUPPORTED_DECRYPT_EXT = {".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx"}
SUPPORTED_ENCRYPT_EXT = {".docx", ".xlsx", ".pptx"}  # OOXML only


class UnsupportedFileError(Exception):
    """Raised when the file extension/format isn't usable for the requested mode."""


class WrongPasswordError(Exception):
    """Raised when decryption fails, almost always due to an incorrect password."""


def get_ext(filename: str) -> str:
    if "." not in filename:
        return ""
    return "." + filename.rsplit(".", 1)[-1].lower()


def check_extension(filename: str, mode: str) -> None:
    """Fast, upfront check based on file extension alone. Raises UnsupportedFileError."""
    ext = get_ext(filename)
    allowed = SUPPORTED_DECRYPT_EXT if mode == "decrypt" else SUPPORTED_ENCRYPT_EXT

    if ext not in allowed:
        if mode == "encrypt" and ext in SUPPORTED_DECRYPT_EXT:
            raise UnsupportedFileError(
                f"'{ext}' files can be decrypted but not encrypted — encryption only "
                f"supports modern Office formats: {', '.join(sorted(SUPPORTED_ENCRYPT_EXT))}."
            )
        raise UnsupportedFileError(
            f"'{ext or 'unknown'}' is not a supported file type for {mode}. "
            f"Supported: {', '.join(sorted(allowed))}."
        )


def is_encrypted(file_path: str) -> bool:
    """Parse the file and report whether it's actually password-protected.

    Also acts as a deeper validity check than the extension alone — raises
    UnsupportedFileError if the file can't be parsed as an Office document at all.
    """
    try:
        with open(file_path, "rb") as f:
            office_file = msoffcrypto.OfficeFile(f)
            return office_file.is_encrypted()
    except UnsupportedFileError:
        raise
    except Exception as e:
        raise UnsupportedFileError(f"Could not read this as an Office file: {e}")


def decrypt_file(input_path: str, output_path: str, password: str) -> None:
    with open(input_path, "rb") as f:
        office_file = msoffcrypto.OfficeFile(f)

        # verify_password is only meaningful for ECMA-376 Agile/Standard encryption;
        # for other formats it's effectively a no-op, so this is safe to always pass.
        try:
            office_file.load_key(password=password, verify_password=True)
        except Exception as e:
            raise WrongPasswordError("Incorrect password.") from e

        with open(output_path, "wb") as out:
            try:
                office_file.decrypt(out)
            except Exception as e:
                raise WrongPasswordError(
                    "Decryption failed — incorrect password or corrupted file."
                ) from e


def encrypt_file(input_path: str, output_path: str, password: str) -> None:
    with open(input_path, "rb") as f:
        ooxml_file = OOXMLFile(f)
        with open(output_path, "wb") as out:
            ooxml_file.encrypt(password, out)
