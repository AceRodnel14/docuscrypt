import logging
import os
import shutil
import tempfile
import time
from pathlib import Path

from fastapi import FastAPI, Form, File, UploadFile, HTTPException
from fastapi.responses import HTMLResponse, JSONResponse, Response
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates
from starlette.requests import Request

from .crypto import (
    check_extension,
    is_encrypted,
    decrypt_file,
    encrypt_file,
    UnsupportedFileError,
    WrongPasswordError,
    SUPPORTED_DECRYPT_EXT,
    SUPPORTED_ENCRYPT_EXT,
)
from .passwords import generate_password

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
logger = logging.getLogger("docuscrypt")

BASE_DIR = Path(__file__).resolve().parent

app = FastAPI(title="DocusCrypt")
app.mount("/static", StaticFiles(directory=BASE_DIR / "static"), name="static")
templates = Jinja2Templates(directory=BASE_DIR / "templates")


def _new_tmpdir() -> str:
    # Container-local scratch space only — no volume mount, nothing shared
    # across requests or pods. Cleaned up explicitly in every code path below.
    return tempfile.mkdtemp(prefix="docuscrypt_")


@app.get("/", response_class=HTMLResponse)
async def index(request: Request):
    return templates.TemplateResponse(
        "index.html",
        {
            "request": request,
            "decrypt_exts": sorted(SUPPORTED_DECRYPT_EXT),
            "encrypt_exts": sorted(SUPPORTED_ENCRYPT_EXT),
        },
    )


@app.get("/healthz")
async def healthz():
    return {"status": "ok"}


@app.get("/api/generate-password")
async def api_generate_password(length: int = 16, symbols: bool = False):
    return {"password": generate_password(length=length, use_symbols=symbols)}


@app.post("/api/check")
def api_check(mode: str = Form(...), file: UploadFile = File(...)):
    """Pre-flight check only — doesn't need a password, just validates the file
    is the right type for the chosen mode (and, for decrypt, is actually encrypted)."""
    logger.info("check start mode=%s filename=%s", mode, file.filename)

    if mode not in ("encrypt", "decrypt"):
        return JSONResponse(status_code=400, content={"ok": False, "error": "Invalid mode."})

    try:
        check_extension(file.filename, mode)
    except UnsupportedFileError as e:
        logger.warning("check rejected filename=%s reason=%s", file.filename, e)
        return JSONResponse(status_code=400, content={"ok": False, "error": str(e)})

    tmpdir = _new_tmpdir()
    try:
        input_path = os.path.join(tmpdir, file.filename)
        with open(input_path, "wb") as f:
            shutil.copyfileobj(file.file, f)

        if mode == "decrypt":
            try:
                encrypted = is_encrypted(input_path)
            except UnsupportedFileError as e:
                logger.warning("check unreadable filename=%s reason=%s", file.filename, e)
                return JSONResponse(status_code=400, content={"ok": False, "error": str(e)})
            if not encrypted:
                logger.info("check filename=%s not encrypted", file.filename)
                return JSONResponse(
                    status_code=400,
                    content={
                        "ok": False,
                        "error": "This file isn't password-protected — nothing to decrypt.",
                    },
                )
        logger.info("check ok filename=%s", file.filename)
        return {"ok": True}
    finally:
        shutil.rmtree(tmpdir, ignore_errors=True)


@app.post("/api/process")
def api_process(
    mode: str = Form(...),
    password: str = Form(...),
    file: UploadFile = File(...),
):
    logger.info("process start mode=%s filename=%s", mode, file.filename)

    if mode not in ("encrypt", "decrypt"):
        raise HTTPException(status_code=400, detail="mode must be 'encrypt' or 'decrypt'.")
    if not password:
        raise HTTPException(status_code=400, detail="Password is required.")

    try:
        check_extension(file.filename, mode)
    except UnsupportedFileError as e:
        logger.warning("process rejected filename=%s reason=%s", file.filename, e)
        raise HTTPException(status_code=400, detail=str(e))

    tmpdir = _new_tmpdir()
    input_path = os.path.join(tmpdir, file.filename)

    stem, ext = os.path.splitext(file.filename)
    suffix = "_decrypted" if mode == "decrypt" else "_encrypted"
    output_filename = f"{stem}{suffix}{ext}"
    output_path = os.path.join(tmpdir, output_filename)

    try:
        with open(input_path, "wb") as f:
            shutil.copyfileobj(file.file, f)
        logger.info(
            "process saved upload path=%s size=%d", input_path, os.path.getsize(input_path)
        )

        t0 = time.monotonic()
        try:
            if mode == "decrypt":
                if not is_encrypted(input_path):
                    raise HTTPException(
                        status_code=400, detail="This file isn't password-protected."
                    )
                decrypt_file(input_path, output_path, password)
            else:
                encrypt_file(input_path, output_path, password)
        except (WrongPasswordError, UnsupportedFileError) as e:
            logger.warning("process failed filename=%s reason=%s", file.filename, e)
            raise HTTPException(status_code=400, detail=str(e))
        except HTTPException:
            raise
        except Exception as e:
            logger.exception("process unexpected error filename=%s", file.filename)
            raise HTTPException(status_code=500, detail=f"Unexpected error: {e}")

        elapsed = time.monotonic() - t0
        out_size = os.path.getsize(output_path)
        logger.info(
            "process crypto done mode=%s filename=%s elapsed=%.2fs out_size=%d",
            mode, file.filename, elapsed, out_size,
        )

        with open(output_path, "rb") as f:
            data = f.read()

        logger.info(
            "process sending response filename=%s bytes=%d", output_filename, len(data)
        )
        return Response(
            content=data,
            media_type="application/octet-stream",
            headers={"Content-Disposition": f'attachment; filename="{output_filename}"'},
        )
    finally:
        shutil.rmtree(tmpdir, ignore_errors=True)
        logger.info("process cleaned up tmpdir=%s", tmpdir)
