CREATE TABLE IF NOT EXISTS proxy_ips (
    ip_address  TEXT PRIMARY KEY,
    ip_type     TEXT NOT NULL CHECK (ip_type IN ('ipv4', 'ipv6')),
    host        TEXT NOT NULL,
    port        INTEGER NOT NULL DEFAULT 443,
    username    TEXT NOT NULL,
    password    TEXT NOT NULL,
    valid_from  TIMESTAMPTZ NOT NULL,
    valid_until TIMESTAMPTZ NOT NULL,
    plan        TEXT NOT NULL DEFAULT 'QUARTERLY',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ensure strict 1:1 account-to-IP assignment.
-- Allows multiple accounts to have empty string (NA), but any assigned IP must be unique.
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_ip_address_unique
ON accounts (ip_address)
WHERE ip_address IS NOT NULL AND ip_address != '';

-- Seed initial proxy IPs
INSERT INTO proxy_ips (ip_address, ip_type, host, port, username, password, valid_from, valid_until, plan)
VALUES
    ('2402:1f00:8302:91e6:6d08:9249:eca8:8252', 'ipv6', 'dc46-mum-01.algoip.in', 443, 'aip_live_1lsqjsm823jhpkqw', 'aip_sec_xdyvcubp4z2jzvledwmdro2l8t4ckaxc', '2026-09-29 19:35:15+05:30', '2026-12-29 19:35:15+05:30', 'QUARTERLY'),
    ('2402:1f00:8302:9102:794b:a90f:c2bb:9293', 'ipv6', 'dc46-mum-01.algoip.in', 443, 'aip_live_4ubp4mve0b5fzbkd', 'aip_sec_ic9rfljxngjh27gbi5my4lyj0qpqcs2l', '2026-09-29 19:35:15+05:30', '2026-12-29 19:35:15+05:30', 'QUARTERLY'),
    ('148.113.41.42', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'aip_live_trnbam1he8ndlerw', 'aip_sec_qxgf0tu7317loli1p44s1d12y4dfgiyh', '2026-09-29 19:35:15+05:30', '2026-12-29 19:35:15+05:30', 'QUARTERLY'),
    ('148.113.41.41', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'aip_live_uco8bc0guf0cj9wu', 'aip_sec_ipyxzu2mrixua0j0lh76ubu5ec66p66o', '2026-09-29 19:35:15+05:30', '2026-12-29 19:35:15+05:30', 'QUARTERLY')
ON CONFLICT (ip_address) DO NOTHING;
