# webhook-validation-bug

Fixture for missing webhook validation.

Expected finding: the Stripe webhook handler parses and dispatches raw payloads without validating the signature header.
