import json
import hmac
import hashlib


def verify_signature(payload: bytes, signature: str, secret: str) -> bool:
    """Verify Stripe webhook signature."""
    expected = hmac.new(secret.encode(), payload, hashlib.sha256).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)


def main(event: dict, context) -> dict:
    """Handle Stripe webhook events."""
    body = event.get("body", "")
    signature = event.get("headers", {}).get("stripe-signature", "")

    # In production, get from environment
    webhook_secret = "whsec_..."

    if not verify_signature(body.encode(), signature, webhook_secret):
        return {"statusCode": 401, "body": "Invalid signature"}

    payload = json.loads(body)
    event_type = payload.get("type")

    handlers = {
        "payment_intent.succeeded": handle_payment_success,
        "payment_intent.failed": handle_payment_failed,
        "customer.subscription.created": handle_subscription_created,
    }

    handler = handlers.get(event_type)
    if handler:
        handler(payload["data"]["object"])

    return {"statusCode": 200, "body": "OK"}


def handle_payment_success(data: dict):
    print(f"Payment succeeded: {data['id']}")


def handle_payment_failed(data: dict):
    print(f"Payment failed: {data['id']}")


def handle_subscription_created(data: dict):
    print(f"Subscription created: {data['id']}")
