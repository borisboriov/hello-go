#!/usr/bin/env python3
"""Проверка Gateway, PostgreSQL, Redis и сохранения данных после перезапуска."""
import datetime
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[1]
BASE = "http://127.0.0.1:8080"


def request(method, path, body=None, status=200):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method,
                                 headers={"Content-Type": "application/json"})
    try:
        response = urllib.request.urlopen(req, timeout=6)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        result = json.load(response)
        assert response.status == status, (method, path, response.status, result)
        return result


def compose(*args):
    return subprocess.check_output(["docker", "compose", "exec", "-T", *args], cwd=ROOT, text=True).strip()


def main():
    global BASE
    # Выбираем свободный порт, не затрагивая уже запущенные сервисы.
    with socket.socket() as check:
        check.bind(("127.0.0.1", 0))
        port = check.getsockname()[1]
    BASE = f"http://127.0.0.1:{port}"
    env = os.environ.copy()
    env["GATEWAY_ADDR"] = f"127.0.0.1:{port}"
    env.setdefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable")
    env.setdefault("REDIS_ADDR", "localhost:6379")
    env.setdefault("REDIS_DB", "0")
    # SQL/Redis-проверки ниже используют локальные сервисы из compose.yaml.
    category = "smoke-" + uuid.uuid4().hex
    year = 3000 + (uuid.uuid4().int % 5000)
    day = datetime.date(year, 6, 10).isoformat()
    older_day = datetime.date(year, 6, 1).isoformat()
    report_path = f"/api/reports/summary?from={older_day}&to={day}"
    key = f"report:summary:{older_day}:{day}"
    process = None
    with tempfile.TemporaryDirectory(prefix="hello-go-smoke-") as directory:
        binary = str(Path(directory) / "gateway")
        subprocess.run(["go", "build", "-o", binary, "."], cwd=ROOT / "gateway", check=True)
        for bad_config, message in [
            ({"DATABASE_URL": "postgres://postgres:postgres@127.0.0.1:1/cashapp?sslmode=disable"}, "connect to PostgreSQL"),
            ({"REDIS_ADDR": "127.0.0.1:1"}, "connect to Redis"),
        ]:
            failure = subprocess.run([binary], env={**env, **bad_config}, capture_output=True, text=True, timeout=10)
            assert failure.returncode != 0 and message in failure.stderr, failure.stderr
            assert "Gateway service started" not in failure.stderr
        with (Path(directory) / "gateway.log").open("w+") as logs:
            def start():
                nonlocal process
                process = subprocess.Popen([binary], env=env, stdout=logs, stderr=logs)
                deadline = time.monotonic() + 10
                while time.monotonic() < deadline:
                    if process.poll() is not None:
                        logs.seek(0)
                        raise AssertionError(logs.read())
                    try:
                        request("GET", "/ping")
                        return
                    except (urllib.error.URLError, TimeoutError):
                        time.sleep(0.1)
                raise AssertionError("Gateway did not start")

            def stop():
                if process and process.poll() is None:
                    process.send_signal(signal.SIGTERM)
                    process.wait(timeout=20)
                    assert process.returncode == 0, process.returncode

            try:
                start()
                request("POST", "/api/budgets", {"category": category, "limit": 500}, 201)
                request("POST", "/api/budgets", {"category": category, "limit": 600}, 201)
                first = request("POST", "/api/transactions", {"category": category, "amount": 125, "date": day}, 201)
                older = request("POST", "/api/transactions", {"category": category, "amount": 50, "date": older_day}, 201)
                assert older["id"] > first["id"] and older["amount"] == 50
                error = request("POST", "/api/transactions", {"category": category, "amount": 426, "date": day}, 409)
                assert error == {"error": "budget exceeded"}
                before = [tx for tx in request("GET", "/api/transactions") if tx["category"] == category]
                assert [tx["id"] for tx in before] == [first["id"], older["id"]]
                budget = [b for b in request("GET", "/api/budgets") if b["category"] == category]
                assert budget == [{"category": category, "limit": 600}]
                sql = f"SELECT count(*), sum(amount) FROM expenses WHERE category='{category}'; SELECT count(*), max(limit_amount) FROM budgets WHERE category='{category}';"
                rows = compose("postgres", "psql", "-U", "postgres", "-d", "cashapp", "-At", "-c", sql)
                assert rows.splitlines() == ["2|1.75", "1|6.00"], rows
                report = request("GET", report_path)
                assert report["categories"] == [{"category": category, "amount": 175}], report
                assert request("GET", report_path) == report
                redis_args = ["redis", "redis-cli", "-n", env["REDIS_DB"]]
                if env.get("REDIS_PASSWORD"):
                    redis_args += ["-a", env["REDIS_PASSWORD"], "--no-auth-warning"]
                ttl = int(compose(*redis_args, "TTL", key))
                assert 0 < ttl <= 30, ttl
                assert json.loads(compose(*redis_args, "GET", key)) == report
                stop()
                start()
                after = [tx for tx in request("GET", "/api/transactions") if tx["category"] == category]
                assert after == before
                assert [b for b in request("GET", "/api/budgets") if b["category"] == category] == budget
                stop()
                logs.seek(0)
                output = logs.read()
                assert f"report cache miss key={key}" in output, output
                assert f"report cache hit key={key}" in output, output
                print("PASS: startup failures, 201/409, SQL rows, sorting, Redis hit/miss + TTL, persistence after restart")
                print(f"Проверочная категория сохранена в БД: {category}")
            finally:
                stop()


if __name__ == "__main__":
    main()
