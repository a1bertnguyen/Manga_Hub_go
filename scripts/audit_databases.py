"""Read-only audit of configured MangaHub SQLite files, without printing user data."""
import argparse
from contextlib import closing
import json
import os
from pathlib import Path
import sqlite3

ROOT = Path(__file__).resolve().parents[1]
MODULE = ROOT / "mangahub-main" / "MangaHub-main"


def audit(backend, catalog):
    report = {"mode": "read-only", "backend_path": str(backend), "catalog_path": str(catalog), "warnings": []}
    with closing(sqlite3.connect(backend.as_uri() + "?mode=ro", uri=True)) as b, closing(sqlite3.connect(catalog.as_uri() + "?mode=ro", uri=True)) as c:
        def count(db, query):
            return db.execute(query).fetchone()[0]
        report["backend_integrity"] = b.execute("PRAGMA quick_check").fetchone()[0]
        report["catalog_integrity"] = c.execute("PRAGMA quick_check").fetchone()[0]
        tables = [row[0] for row in b.execute("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")]
        report["backend_table_counts"] = {table: count(b, 'SELECT COUNT(*) FROM "' + table.replace('"', '""') + '"') for table in tables}
        report["catalog_manga_count"] = count(c, "SELECT COUNT(*) FROM manga")
        report["foreign_key_violations"] = len(b.execute("PRAGMA foreign_key_check").fetchall())
        users = {row[0] for row in b.execute("SELECT id FROM users")}
        manga = {row[0]: row[1] for row in c.execute("SELECT id,total_chapters FROM manga")}
        progress = b.execute("SELECT user_id,manga_id,current_chapter,status FROM user_progress").fetchall()
        report["progress_without_user"] = sum(user not in users for user, _, _, _ in progress)
        report["progress_missing_from_authoritative_catalog"] = sum(m not in manga for _, m, _, _ in progress)
        report["progress_orphan_rows"] = sum(user not in users or m not in manga for user, m, _, _ in progress)
        report["progress_outside_chapter_bounds"] = sum(ch < 0 or (m in manga and ch > manga[m]) for _, m, ch, _ in progress)
        report["progress_unknown_status"] = sum(status not in {"reading", "completed", "plan_to_read", "on-hold", "dropped"} for _, _, _, status in progress)
        report["non_bcrypt_password_rows"] = count(b, "SELECT COUNT(*) FROM users WHERE password_hash NOT LIKE '$2a$%' AND password_hash NOT LIKE '$2b$%' AND password_hash NOT LIKE '$2y$%'")
        report["catalog_negative_chapters"] = count(c, "SELECT COUNT(*) FROM manga WHERE total_chapters < 0")
        report["chat_schema_ready"] = "chat_messages" in tables
        if "chat_messages" in tables:
            report["chat_without_user"] = count(b, "SELECT COUNT(*) FROM chat_messages cm LEFT JOIN users u ON cm.user_id=u.id WHERE u.id IS NULL")
        for key in ("foreign_key_violations", "progress_without_user", "progress_missing_from_authoritative_catalog", "progress_outside_chapter_bounds", "progress_unknown_status", "non_bcrypt_password_rows", "catalog_negative_chapters", "chat_without_user"):
            if report.get(key, 0):
                report["warnings"].append(f"{key}: {report[key]}")
        if not report["chat_schema_ready"]:
            report["warnings"].append("chat_messages missing: start the updated backend to run the additive schema migration")
    report["clean"] = not report["warnings"] and report["backend_integrity"] == report["catalog_integrity"] == "ok"
    return report


def main():
    settings = {}
    local = MODULE / ".env.local"
    if local.exists():
        for line in local.read_text(encoding="utf-8-sig").splitlines():
            if line.strip() and not line.lstrip().startswith("#") and "=" in line:
                key, value = line.split("=", 1)
                settings[key.strip()] = value.strip().strip('"\'')
    def configured(key, fallback):
        path = Path(os.environ.get(key, settings.get(key, fallback)))
        return (path if path.is_absolute() else MODULE / path).resolve()
    parser = argparse.ArgumentParser()
    parser.add_argument("--backend", type=Path, default=configured("BACKEND_DB_PATH", "data/mangahub.db"))
    parser.add_argument("--catalog", type=Path, default=configured("CATALOG_DB_PATH", "data/catalog.db"))
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    result = audit(args.backend.resolve(), args.catalog.resolve())
    output = json.dumps(result, indent=2) + "\n"
    print(output)
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(output, encoding="utf-8")
    return 0 if result["clean"] else 2


if __name__ == "__main__":
    raise SystemExit(main())
