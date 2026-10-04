# Your data

You can take a copy of what Ignition holds about you and you can have it erased. Both are on the Settings page of the app, and both are also available through the [API](/docs/api).

## Exporting your data

Choose "Export my data". Your browser downloads a JSON file named `ignition-export-<date>.json`. It contains:

- your account: its id, email address and when it was created,
- your memberships: each workspace you belong to and your role in it,
- the pending invitations addressed to your email address.

It never contains your password hash, session tokens or any other secret.

## Deleting your account

Choose "Delete account" and confirm with your password. A wrong password is refused without signing you out.

When the deletion goes through:

- the workspaces you own alone are erased, with their members' roles, invitations, plan and billing records,
- workspaces you only belong to are left, and you are removed from them,
- invitations addressed to your email address are deleted,
- your sessions and your account are deleted.

Erasure is permanent. A request that fails part way can be retried safely.

## What can block a deletion

The server checks these before it removes anything, so a refusal changes nothing:

1. **You own a workspace that still has other members.** Transfer ownership to someone else, or remove the other members, then try again. The message is "transfer ownership of your shared workspaces first".
2. **A workspace you own alone is still billed.** A workspace whose subscription is active, trialing or past due blocks deletion, because erasing it does not cancel anything at Stripe. Cancel the subscription in the customer portal first. The message is "cancel your workspace subscriptions first".

Both come back as `failed_precondition`.
