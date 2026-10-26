# Mainland China network data

The website access gate embeds `china.txt` (IPv4) and `china6.txt` (IPv6).
These are local country network lists; no visitor IP is sent to a third party.
They are used only by the website access decision, never by API authentication
or gateway routing. Network registration/geolocation data can be imperfect;
an IP match describes a network, not a person's nationality or physical location.

- Source: https://github.com/gaoyifan/china-operator-ip
- Data branch: `ip-lists`
- Pinned commit: `ca7016c1712529bfa118efb993fc576e5f0d26ed`
- Upstream commit time: `2026-10-25T07:53:52Z`
- License: MIT; the upstream notice is included in `LICENSE`.
- `china.txt` SHA-256: `9407e9e63c347a4196bdb46765d873c0cb326f67673ae1c081efbee38c576a1c`
- `china6.txt` SHA-256: `56462a0a1b70a6889a95beac700e657fa6eaa9967597f6986382d8b055d7fcc8`

To refresh, choose and record an upstream commit, replace both lists from that
same commit, update this provenance and hashes, run the website access tests,
and rebuild the backend. Keep the license notice. Review unexpected large
changes before deploying; the running service does not auto-download data.

Pinned file URLs:

- https://raw.githubusercontent.com/gaoyifan/china-operator-ip/ca7016c1712529bfa118efb993fc576e5f0d26ed/china.txt
- https://raw.githubusercontent.com/gaoyifan/china-operator-ip/ca7016c1712529bfa118efb993fc576e5f0d26ed/china6.txt
