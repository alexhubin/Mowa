#!/usr/bin/env python3
"""Create a PostgreSQL import transaction from a legacy Mova SQLite database."""

from __future__ import annotations

import argparse
import os
import re
import sqlite3
from pathlib import Path


def quote(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def username_for(email: str, used: set[str]) -> str:
    base = re.sub(r"[^a-z0-9_]", "_", email.split("@", 1)[0].lower()).strip("_")
    if len(base) < 3:
        base = f"user_{base}".rstrip("_")
    base = base[:32]
    candidate = base
    suffix = 2
    while candidate in used:
        tail = f"_{suffix}"
        candidate = base[: 32 - len(tail)] + tail
        suffix += 1
    used.add(candidate)
    return candidate


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("sqlite_database", type=Path)
    parser.add_argument("output_sql", type=Path)
    args = parser.parse_args()

    source = args.sqlite_database.resolve(strict=True)
    output = args.output_sql.resolve()
    if source == output:
        raise SystemExit("input and output paths must differ")

    connection = sqlite3.connect(f"file:{source}?mode=ro", uri=True)
    connection.row_factory = sqlite3.Row
    users = connection.execute("SELECT id, email, display_name, password_hash, created_at FROM users ORDER BY created_at, id").fetchall()
    sessions = connection.execute("SELECT token_hash, user_id, expires_at, created_at FROM sessions").fetchall()
    rooms = connection.execute("SELECT id, invite_code, name, owner_id, created_at FROM rooms ORDER BY created_at, id").fetchall()
    connection.close()

    used: set[str] = set()
    flags = os.O_WRONLY | os.O_CREAT | os.O_TRUNC
    descriptor = os.open(output, flags, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as target:
        target.write("BEGIN;\n")
        for user in users:
            username = username_for(user["email"], used)
            target.write(
                "INSERT INTO users (id, username, email, display_name, password_hash, created_at, updated_at) VALUES "
                f"({quote(user['id'])}, {quote(username)}, {quote(user['email'])}, {quote(user['display_name'])}, "
                f"{quote(user['password_hash'])}, to_timestamp({int(user['created_at'])}), to_timestamp({int(user['created_at'])}));\n"
            )
            target.write(
                "INSERT INTO user_settings (user_id, video_quality, updated_at) VALUES "
                f"({quote(user['id'])}, 'high', to_timestamp({int(user['created_at'])}));\n"
            )
        for session in sessions:
            target.write(
                "INSERT INTO sessions (token_hash, user_id, expires_at, created_at) VALUES "
                f"({quote(session['token_hash'])}, {quote(session['user_id'])}, to_timestamp({int(session['expires_at'])}), "
                f"to_timestamp({int(session['created_at'])}));\n"
            )
        for room in rooms:
            target.write(
                "INSERT INTO rooms (id, invite_code, name, owner_id, kind, created_at) VALUES "
                f"({quote(room['id'])}, {quote(room['invite_code'])}, {quote(room['name'])}, {quote(room['owner_id'])}, "
                f"'group', to_timestamp({int(room['created_at'])}));\n"
            )
            target.write(
                "INSERT INTO room_members (room_id, user_id, created_at) VALUES "
                f"({quote(room['id'])}, {quote(room['owner_id'])}, to_timestamp({int(room['created_at'])}));\n"
            )
        target.write("COMMIT;\n")

    print(f"prepared {len(users)} users, {len(sessions)} sessions and {len(rooms)} rooms")


if __name__ == "__main__":
    main()
