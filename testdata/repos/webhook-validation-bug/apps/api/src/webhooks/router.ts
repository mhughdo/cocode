import { handleStripeWebhook } from "./stripe";

export function registerWebhookRoutes(router) {
  router.post("/webhooks/stripe", handleStripeWebhook);
}
