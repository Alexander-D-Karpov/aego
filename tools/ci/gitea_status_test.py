import contextlib
import io
import os
import unittest
from unittest.mock import patch

import gitea_status as status


ENV = {
    "GITHUB_SERVER_URL": "https://github.com",
    "GITHUB_REPOSITORY": "Alexander-D-Karpov/aego",
    "GITHUB_REF": "refs/heads/master",
    "GITHUB_EVENT_NAME": "push",
    "GITHUB_SHA": "a" * 40,
    "GITHUB_RUN_ID": "42",
    "GITHUB_RUN_ATTEMPT": "2",
    "GITEA_STATUS_TOKEN": "not-a-real-token",
    "GITHUB_TOKEN": "not-a-real-github-token",
}


class StatusTests(unittest.TestCase):
    @patch.dict(os.environ, ENV, clear=True)
    @patch.object(status, "request_json")
    def test_pending_posts_one_status_per_platform(self, request):
        self.assertEqual(status.main(["pending"]), 0)
        self.assertEqual(request.call_count, 4)
        self.assertEqual({c.args[2]["context"] for c in request.call_args_list}, {"GitHub / " + n for n in status.JOBS})
        self.assertTrue(all(c.args[2]["state"] == "pending" for c in request.call_args_list))

    @patch.dict(os.environ, ENV, clear=True)
    @patch.object(status, "request_json")
    def test_finished_uses_actual_job_conclusions(self, request):
        jobs = [{"name": name, "id": i + 1, "status": "completed", "conclusion": result} for i, (name, result) in enumerate(zip(status.JOBS, ["success", "failure", "cancelled", "success"]))]
        request.side_effect = [{"jobs": jobs}, {}, {}, {}, {}]
        self.assertEqual(status.main(["finished"]), 0)
        self.assertIn("/jobs?filter=all&", request.call_args_list[0].args[0])
        posts = [call.args[2] for call in request.call_args_list[1:]]
        self.assertEqual([p["state"] for p in posts], ["success", "failure", "error", "success"])
        self.assertTrue(posts[1]["target_url"].endswith("/runs/42/job/2"))

    @patch.dict(os.environ, ENV, clear=True)
    @patch.object(status, "request_json")
    def test_failed_job_rerun_keeps_earlier_successes(self, request):
        jobs = [{"name": name, "id": i + 1, "run_attempt": 1, "status": "completed", "conclusion": "success"} for i, name in enumerate(status.JOBS)]
        jobs.append({"name": status.JOBS[1], "id": 10, "run_attempt": 2, "status": "completed", "conclusion": "failure"})
        request.side_effect = [{"jobs": jobs}, {}, {}, {}, {}]
        self.assertEqual(status.main(["finished"]), 0)
        posts = [call.args[2] for call in request.call_args_list[1:]]
        self.assertEqual([p["state"] for p in posts], ["success", "failure", "success", "success"])

    @patch.dict(os.environ, ENV, clear=True)
    @patch.object(status, "request_json")
    def test_missing_job_is_never_reported_as_success(self, request):
        request.side_effect = [{"jobs": []}, {}, {}, {}, {}]
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(status.main(["finished"]), 1)
        self.assertTrue(all(call.args[2]["state"] == "error" for call in request.call_args_list[1:]))

    @patch.dict(os.environ, {**ENV, "GITHUB_EVENT_NAME": "pull_request"}, clear=True)
    @patch.object(status, "request_json")
    def test_untrusted_events_never_send_tokens(self, request):
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(status.main(["pending"]), 0)
        request.assert_not_called()

    @patch.dict(os.environ, {**ENV, "GITEA_STATUS_TOKEN": ""}, clear=True)
    @patch.object(status, "request_json")
    def test_missing_secret_does_not_change_ci_result(self, request):
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(status.main(["pending"]), 0)
        request.assert_not_called()


if __name__ == "__main__":
    unittest.main()
