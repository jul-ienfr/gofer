#!/usr/bin/env python3
"""Populate a disposable Gofer database with synthetic load-test data.

The target database must already have the Gofer schema. The script reuses the
first user (or creates `default`), then replaces the `bench-account` data set:

- messages across Inbox, Archive and Sent, with folder/thread state and search
  rows;
- contact profiles and email fields;
- one CalDAV calendar source with events around the current month.

Run it against a copy, never production:

    python perf/seed_load.py --db %TEMP%\\gofer-load\\gofer.db
    python perf/seed_load.py --db /tmp/gofer-load/gofer.db --messages 50000
"""

import argparse
import datetime as dt
import sqlite3
import sys

ACCOUNT_ID = "bench-account"
INBOX = "bench-inbox"
ARCHIVE = "bench-archive"
SENT = "bench-sent"
CALENDAR_ID = "bench-calendar"


def first_user(conn):
    row = conn.execute(
        "SELECT id FROM users WHERE status = 'active' ORDER BY created_at, id LIMIT 1"
    ).fetchone()
    if row:
        return row[0]
    conn.execute(
        "INSERT INTO users(id, username, username_normalized, name) VALUES ('default', 'local', 'local', 'Local')"
    )
    return "default"


def reset_bench(conn):
    conn.execute("DELETE FROM message_search WHERE account_id = ?", (ACCOUNT_ID,))
    conn.execute("DELETE FROM accounts WHERE id = ?", (ACCOUNT_ID,))
    conn.execute("DELETE FROM contact_profiles WHERE id LIKE 'bench-contact-%'")
    conn.execute("DELETE FROM calendar_sources WHERE id = ?", (CALENDAR_ID,))
    conn.execute("DELETE FROM threads WHERE id LIKE 'bench-thread-%'")


def seed_mail(conn, user_id, messages):
    start = dt.datetime(2015, 1, 1)
    account = (
        ACCOUNT_ID,
        user_id,
        "imap",
        "reader@example.test",
        "Reader",
        "#336699",
        "R",
    )
    conn.execute(
        """INSERT INTO accounts(id, user_id, provider, email_address, display_name, color, initials,
             email_sync_enabled, provider_push_enabled)
           VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0)""",
        account,
    )
    conn.executemany(
        """INSERT INTO folders(id, account_id, remote_id, name, role, sort_order)
           VALUES (?, ?, ?, ?, ?, ?)""",
        [
            (INBOX, ACCOUNT_ID, "INBOX", "Inbox", "inbox", 0),
            (ARCHIVE, ACCOUNT_ID, "Archive", "Archive", "archive", 4),
            (SENT, ACCOUNT_ID, "Sent", "Sent", "sent", 2),
        ],
    )

    message_rows = []
    folder_rows = []
    search_rows = []
    recipient_rows = []
    for i in range(1, messages + 1):
        sent_at = (start + dt.timedelta(minutes=15 * i)).strftime("%Y-%m-%d %H:%M:%S")
        thread = f"bench-thread-{i // 3}"
        subject = f"Subject {i % 5000}"
        sender_name = f"Sender {i % 2000}"
        sender_email = f"sender{i % 2000}@example.test"
        snippet = f"A short preview of message {i}"
        has_attachments = 1 if i % 10 == 0 else 0
        message_rows.append(
            (
                i,
                ACCOUNT_ID,
                f"<m{i}@example.test>",
                f"m{i}@example.test",
                thread,
                subject,
                subject.lower(),
                sender_name,
                sender_email,
                sent_at,
                sent_at,
                snippet,
                snippet,
                2000 + i % 90000,
                has_attachments,
            )
        )
        folder = INBOX if i % 10 < 6 else (ARCHIVE if i % 10 < 9 else SENT)
        folder_rows.append(
            (
                i,
                folder,
                i,
                0 if i % 17 == 0 else 1,
                1 if i % 97 == 0 else 0,
            )
        )
        body = (
            f"Hello, this is message {i}"
            + (
                " about the invoice for last month. "
                if i % 3 == 0
                else ". "
            )
            + "The team met on Tuesday to review the plan, the budget and the open questions."
            * 6
        )
        search_rows.append(
            (
                i,
                ACCOUNT_ID,
                thread,
                subject,
                f"{sender_name} {sender_email}",
                "Reader reader@example.test",
                snippet,
                body,
                "",
            )
        )
        recipient_rows.append((i, "to", "Reader", "reader@example.test"))

    conn.executemany(
        """INSERT INTO messages(id, account_id, internet_message_id, message_id_normalized,
             thread_id, subject, normalized_subject, from_name, from_email, date_sent,
             date_received, snippet, preview_text, size_bytes, has_attachments)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
        message_rows,
    )
    conn.executemany(
        """INSERT INTO message_folder_state(message_id, folder_id, remote_uid, is_read, is_starred, synced_at)
           VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)""",
        folder_rows,
    )
    conn.executemany(
        """INSERT INTO message_recipients(message_id, kind, name, email) VALUES (?, ?, ?, ?)""",
        recipient_rows,
    )
    conn.executemany(
        """INSERT INTO message_search(rowid, account_id, thread_key, subject, sender, recipients,
             snippet, body, attachment_names) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)""",
        search_rows,
    )
    conn.executemany(
        """INSERT INTO folder_thread_state(folder_id, thread_key, head_message_id, account_id,
             last_message_at, thread_count, thread_is_read, thread_is_starred, thread_has_attachments)
           SELECT message_folder_state.folder_id, messages.thread_id, MAX(messages.id), messages.account_id,
             MAX(messages.date_received), COUNT(*), MIN(message_folder_state.is_read),
             MAX(message_folder_state.is_starred), MAX(messages.has_attachments)
           FROM messages JOIN message_folder_state ON message_folder_state.message_id = messages.id
           WHERE messages.account_id = ?
           GROUP BY message_folder_state.folder_id, messages.thread_id""",
        [(ACCOUNT_ID,)],
    )
    conn.executemany(
        """INSERT INTO threads(id, account_id, subject, normalized_subject, root_message_id,
             last_message_at, message_count, unread_count)
           SELECT messages.thread_id, messages.account_id, MIN(messages.subject),
             MIN(messages.normalized_subject), MAX(messages.id), MAX(messages.date_received),
             COUNT(*), SUM(CASE WHEN message_folder_state.is_read = 0 THEN 1 ELSE 0 END)
           FROM messages JOIN message_folder_state ON message_folder_state.message_id = messages.id
           WHERE messages.account_id = ?
           GROUP BY messages.thread_id""",
        [(ACCOUNT_ID,)],
    )
    conn.execute(
        """UPDATE folders SET
             total_count = (SELECT COUNT(*) FROM message_folder_state WHERE folder_id = folders.id),
             unread_count = (SELECT COUNT(*) FROM message_folder_state WHERE folder_id = folders.id AND is_read = 0)
           WHERE account_id = ?""",
        (ACCOUNT_ID,),
    )


def seed_contacts(conn, user_id, contacts):
    profiles = []
    fields = []
    for i in range(1, contacts + 1):
        profile = f"bench-contact-{i}"
        email = f"contact{i}@example.test"
        profiles.append((profile, user_id, f"Contact {i}", f"Contact {i}", email))
        fields.append(
            (
                f"bench-field-{i}",
                user_id,
                profile,
                "email",
                "",
                email,
                email.lower(),
                1,
                0,
                "manual",
            )
        )
    conn.executemany(
        """INSERT INTO contact_profiles(id, user_id, display_name, sort_name, primary_email, origin)
           VALUES (?, ?, ?, ?, ?, 'manual')""",
        profiles,
    )
    conn.executemany(
        """INSERT INTO contact_fields(id, user_id, profile_id, kind, label, value, normalized_value,
             is_primary, ordinal, source)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
        fields,
    )


def seed_calendar(conn, user_id, events):
    conn.execute(
        """INSERT INTO calendar_sources(id, user_id, account_id, provider, remote_id, name,
             timezone, color, access_role, is_primary, is_selected)
           VALUES (?, ?, ?, 'caldav', 'bench-calendar', 'Bench calendar', 'UTC', '#336699', 'owner', 1, 1)""",
        (CALENDAR_ID, user_id, ACCOUNT_ID),
    )
    start = dt.datetime.now().replace(day=1, hour=9, minute=0, second=0, microsecond=0)
    rows = []
    for i in range(1, events + 1):
        begins = start + dt.timedelta(hours=6 * i)
        ends = begins + dt.timedelta(minutes=45)
        rows.append(
            (
                f"bench-event-{i}",
                user_id,
                CALENDAR_ID,
                f"event-{i}",
                f"bench-event-{i}@example.test",
                f"Bench event {i}",
                "",
                "Bench room",
                "Bench owner",
                "owner@example.test",
                begins.strftime("%Y-%m-%d %H:%M:%S"),
                ends.strftime("%Y-%m-%d %H:%M:%S"),
                "UTC",
                "UTC",
            )
        )
    conn.executemany(
        """INSERT INTO calendar_events(id, user_id, source_id, remote_id, ical_uid, summary,
             description, location, organizer_name, organizer_email, start_at, end_at,
             start_timezone, end_timezone, attendees_json, online_meeting_json)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', '{}')""",
        rows,
    )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", required=True)
    parser.add_argument("--messages", type=int, default=10000)
    parser.add_argument("--contacts", type=int, default=1000)
    parser.add_argument("--events", type=int, default=500)
    args = parser.parse_args()
    if min(args.messages, args.contacts, args.events) < 0:
        parser.error("counts must not be negative")

    conn = sqlite3.connect(args.db)
    try:
        conn.execute("PRAGMA foreign_keys = ON")
        conn.execute("BEGIN")
        user_id = first_user(conn)
        reset_bench(conn)
        seed_mail(conn, user_id, args.messages)
        seed_contacts(conn, user_id, args.contacts)
        seed_calendar(conn, user_id, args.events)
        conn.commit()
        print(
            f"seeded user={user_id} messages={args.messages} contacts={args.contacts} events={args.events}"
        )
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()


if __name__ == "__main__":
    sys.exit(main())
