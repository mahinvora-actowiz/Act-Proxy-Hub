#!/usr/bin/env python3
"""Backfill isError in daily MongoDB log collections.

Dry run by default. Pass --apply to write changes.
Requires pymongo and python-dotenv (pip install pymongo python-dotenv).
"""

import argparse
import os
from datetime import date, timedelta

from dotenv import load_dotenv
from pymongo import MongoClient


def date_range(start: date, end: date):
    current = start
    while current <= end:
        yield current
        current += timedelta(days=1)


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Add isError to historical daily log documents."
    )
    parser.add_argument("--start", type=date.fromisoformat, default=date(2026, 6, 3))
    parser.add_argument("--end", type=date.fromisoformat, default=date(2026, 11, 17))
    parser.add_argument(
        "--apply",
        action="store_true",
        help="Write updates to MongoDB. Without this flag, only report planned counts.",
    )
    args = parser.parse_args()

    if args.end < args.start:
        parser.error("--end must be on or after --start")

    load_dotenv()
    mongo_uri = os.getenv("MONGO_URI")
    db_name = os.getenv("DB_NAME")
    if not mongo_uri or not db_name:
        parser.error("MONGO_URI and DB_NAME must be set in the environment or .env")

    print(f"Mode: {'APPLY' if args.apply else 'DRY RUN'}")
    print(f"Date range: {args.start} through {args.end} (inclusive)")

    total_missing = 0
    total_updated = 0

    with MongoClient(mongo_uri) as client:
        db = client[db_name]
        existing_collections = set(db.list_collection_names())

        for day in date_range(args.start, args.end):
            collection_name = f"logs_{day:%Y_%m_%d}"
            if collection_name not in existing_collections:
                print(f"{collection_name}: missing, skipped")
                continue

            collection = db[collection_name]
            query = {"isError": {"$exists": False}}
            missing_count = collection.count_documents(query)
            updated_count = 0

            if args.apply and missing_count:
                # Matches the Go logging rule: only status 200 is not an error.
                result = collection.update_many(
                    query,
                    [{"$set": {"isError": {"$ne": ["$statusCode", 200]}}}],
                )
                updated_count = result.modified_count

            total_missing += missing_count
            total_updated += updated_count
            suffix = f", updated {updated_count}" if args.apply else ""
            print(f"{collection_name}: {missing_count} documents need isError{suffix}")

    print(f"Total documents missing isError: {total_missing}")
    if args.apply:
        print(f"Total documents updated: {total_updated}")
    else:
        print("No database changes made. Re-run with --apply to write these updates.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
