import sqlite3
import time

# Connect to the old and new databases
old_conn = sqlite3.connect("bot-old.db")
new_conn = sqlite3.connect("bot.db")

old_cursor = old_conn.cursor()
new_cursor = new_conn.cursor()

# Create the new table with the updated schema
new_cursor.execute("""
CREATE TABLE IF NOT EXISTS monitored_users (
    guild_id TEXT,
    user_id TEXT,
    username TEXT,
    notification_channel TEXT,
    last_post_id TEXT,
    last_stream_start INTEGER,
    mention_role TEXT,
    avatar_location TEXT,
    avatar_location_updated_at INTEGER,
    PRIMARY KEY (guild_id, user_id)
)
""")

# Fetch all records from the old table
old_cursor.execute("SELECT * FROM monitored_users")
old_records = old_cursor.fetchall()

# Insert the old records into the new table
for record in old_records:
    # Assuming the order of columns in the old table is:
    # guild_id, user_id, username, notification_channel, last_post_id, last_stream_start, mention_role, avatar_location
    new_cursor.execute(
        """
    INSERT INTO monitored_users 
    (guild_id, user_id, username, notification_channel, last_post_id, last_stream_start, mention_role, avatar_location, avatar_location_updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    """,
        (*record, int(time.time())),
    )  # Add current timestamp for avatar_location_updated_at

# Commit the changes and close the connections
new_conn.commit()
old_conn.close()
new_conn.close()

print("Migration completed successfully.")
