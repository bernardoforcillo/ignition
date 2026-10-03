# Plans and billing

Plans belong to workspaces. Each workspace is on exactly one plan at a time and can add add-ons on top.

## Free and paid plans

Every new workspace starts on the Free plan. The starter catalog that ships with Ignition defines:

| Plan | API calls per month | Notes |
| --- | --- | --- |
| Free | 1,000 | The plan every workspace starts on |
| Pro | 100,000 | Includes everything in Free and the data export feature |

An add-on tops up a plan. The starter catalog has one, Extra API calls, which adds 50,000 calls per month to a workspace on Pro.

These names, limits and prices are the template's defaults. A deployment replaces them with its own catalog, and the pricing section of the landing page lists whichever paid plans the deployment has switched on.

## Upgrading

Owners and admins open the Billing page of the workspace and choose a plan or an add-on. Ignition sends them to a Stripe hosted checkout page. When the payment succeeds, Stripe tells Ignition with a signed message and the workspace moves to the new plan. The Billing page then shows the plan, its status and the end of the current period.

Card details are entered on Stripe's page and never reach Ignition.

## Managing a subscription

Once a workspace has a subscription, "Manage billing" opens the Stripe customer portal, where an owner or admin can update the payment method, download invoices and cancel.

## Cancelling

When a subscription ends, Stripe reports it and the workspace returns to the Free plan. Nothing is deleted: members, invitations and data stay as they were. A workspace that was cancelled can still open the customer portal to look at its invoices or subscribe again.

## When billing is not enabled

Billing is optional. A deployment without Stripe configured does not offer paid plans, the Billing page says that billing is not enabled, and the pricing section shows only the Free plan.
