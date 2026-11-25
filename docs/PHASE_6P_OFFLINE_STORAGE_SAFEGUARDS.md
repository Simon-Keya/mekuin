# Phase 6P — Browser storage pressure and persistence safeguards

## Implemented

- POS displays browser storage usage/quota estimates when the Storage API exposes them.
- POS displays whether the browser reports storage as persistent.
- An explicit operator action requests persistent storage; no prompt is initiated automatically.
- Unsupported or denied Storage API calls degrade gracefully.
- Pending/syncing/blocked drafts are never automatically deleted to free storage.
- Existing quota and IndexedDB errors remain visible; synced records can be manually cleared by the operator.

## Limitations

Storage estimates are approximate. A persistence request can be denied and even granted persistence does not protect against the user clearing site data, profile deletion, device loss, browser bugs, or hardware failure. This feature is not a backup. Do not store sensitive personal information in draft labels.
