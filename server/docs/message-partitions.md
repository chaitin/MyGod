# Message partitions

The `messages` table is partitioned first by UTC calendar year and then by a 32-way hash of `conversation_id`.

```text
messages
└── messages_2026
    ├── messages_2026_p00
    ├── ...
    └── messages_2026_p31
```

All annual partitions remain attached to `messages`. PostgreSQL partition pruning keeps normal message queries away from years outside the online window; there is no separate archive schema or runtime archive routing.

`message_registry` retains the global message ID, conversation sequence, client idempotency key, reply location, visibility metadata, and partition year. Message bodies remain in their year/hash partitions.

## Online history window

Normal history, reply, forwarding, revoke, and app trigger queries are limited to the current and previous UTC calendar years. The retention is deliberately defined in code by `store.MessageOnlineRetentionYears` rather than runtime configuration.

For example, during 2026:

```text
2025 and 2026  online and queryable
2024 or older  retained in PostgreSQL but excluded from normal history operations
```

Changing the code constant from two years to three makes the current and previous two UTC years queryable; no partition restore is required because old annual partitions stay attached.

The server bootstrap ensures that every year in the online window plus the next two UTC years exists. Existing legacy years are created and populated by migration `00015_partition_messages_by_year_and_conversation.sql`.

## Deployment safety

Migration `00015_partition_messages_by_year_and_conversation.sql` converts the existing unpartitioned table and copies its rows in one PostgreSQL transaction. Deploy it before the message table becomes large, or run the deployment during a maintenance window. For an already large production table, use a shadow-table backfill and short cutover instead of running the startup migration directly.

Old partitions continue to consume primary database storage and remain part of database backups. A separate cold-storage lifecycle can be added later if storage cost requires it.

Global client-message idempotency still uses `message_registry` across every retained year so that an old retry cannot create a duplicate message. This point lookup is separate from the normal history window.

Do not modify `created_at`, `conversation_id`, `seq`, sender identity, or the client message ID after insertion. They are immutable routing and identity fields.
