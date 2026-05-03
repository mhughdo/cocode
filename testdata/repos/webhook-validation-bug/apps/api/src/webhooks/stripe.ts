import { billingEvents } from "../services/billing-events";

export async function handleStripeWebhook(request, response) {
  const event = JSON.parse(request.rawBody.toString("utf8"));

  if (event.type === "invoice.paid") {
    await billingEvents.markInvoicePaid(event.data.object.id);
  }

  response.status(204).end();
}
