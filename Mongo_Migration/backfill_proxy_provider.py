#!/usr/bin/env python3
"""Backfill proxyProvider on daily MongoDB request log collections.

Dry run by default. Pass --apply to write changes.
Requires pymongo and python-dotenv (pip install pymongo python-dotenv).
"""

import argparse
import os
from datetime import date, timedelta

from bson import ObjectId
from dotenv import load_dotenv
from pymongo import MongoClient


TOKEN_PROVIDERS = {
    ObjectId("6a1d92d9437426b98a486f94"): "scrapedo",
    ObjectId("6a2118ca85fdc70c9c4f73c1"): "scraperapi",
    ObjectId("6aacef813669900c45d4d9a4"): "actone",
}


def date_range(start: date, end: date):
    current = start
    while current <= end:
        yield current
        current += timedelta(days=1)


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Add proxyProvider to matching historical daily log documents."
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

    # Load the same local .env file used by the Go service, without printing values.
    load_dotenv()
    mongo_uri = os.getenv("MONGO_URI")
    db_name = os.getenv("DB_NAME")
    if not mongo_uri or not db_name:
        parser.error("MONGO_URI and DB_NAME must be set in the environment or .env")

    mode = "APPLY" if args.apply else "DRY RUN"
    print(f"Mode: {mode}")
    print(f"Date range: {args.start} through {args.end} (inclusive)")

    total_matched = 0
    total_updated = 0
    total_unmapped = 0

    with MongoClient(mongo_uri) as client:
        db = client[db_name]
        existing_collections = set(db.list_collection_names())

        for day in date_range(args.start, args.end):
            collection_name = f"logs_{day:%Y_%m_%d}"
            if collection_name not in existing_collections:
                print(f"{collection_name}: missing, skipped")
                continue

            collection = db[collection_name]
            collection_matched = 0
            collection_updated = 0

            # Only fill absent fields; preserve any proxyProvider already present.
            for token_id, provider in TOKEN_PROVIDERS.items():
                query = {"tokenId": token_id, "proxyProvider": {"$exists": False}}
                count = collection.count_documents(query)
                collection_matched += count
                if args.apply and count:
                    result = collection.update_many(
                        query, {"$set": {"proxyProvider": provider}}
                    )
                    collection_updated += result.modified_count

            unmapped_query = {
                "proxyProvider": {"$exists": False},
                "tokenId": {"$nin": list(TOKEN_PROVIDERS)},
            }
            unmapped_count = collection.count_documents(unmapped_query)
            total_unmapped += unmapped_count
            total_matched += collection_matched
            total_updated += collection_updated

            suffix = f", updated {collection_updated}" if args.apply else ""
            print(
                f"{collection_name}: {collection_matched} matching documents{suffix}; "
                f"{unmapped_count} documents have an unmapped or missing tokenId"
            )

    print(f"Total matching documents: {total_matched}")
    if args.apply:
        print(f"Total documents updated: {total_updated}")
    else:
        print("No database changes made. Re-run with --apply to write these updates.")
    print(f"Unmapped documents left unchanged: {total_unmapped}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
