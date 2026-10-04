-- Run through psql on the server. Optional username filter: replace the WHERE line.
SELECT d.received_at, u.username, d.call_id, d.payload
FROM desktop_diagnostics d JOIN users u ON u.id=d.user_id
WHERE d.received_at > now()-interval '14 days'
ORDER BY d.received_at DESC LIMIT 100;
