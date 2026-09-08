import unittest

from handler import main, verify_signature


class WebhookTest(unittest.TestCase):
    def test_rejects_bad_signature(self):
        self.assertFalse(verify_signature(b"{}", "invalid", "secret"))
        self.assertEqual(main({"body": "{}"}, None)["statusCode"], 401)


if __name__ == "__main__":
    unittest.main()
