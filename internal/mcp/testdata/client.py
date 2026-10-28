"""Synthetic OAuth and MCP smoke test, driven by TestPythonClientInterop."""

import asyncio
import base64
from datetime import timedelta
import hashlib
import html
import os
import re
from urllib.parse import parse_qs, urlparse

import httpx
from mcp import ClientSession, McpError
from mcp.client.streamable_http import streamable_http_client


async def main():
    base = os.environ["JULIUS_MCP_TEST_URL"]
    resource = base + "/mcp"
    async with httpx.AsyncClient(base_url=base, timeout=10, trust_env=False) as http:
        response = await http.post("/mcp", json={})
        assert response.status_code == 401
        assert base + "/.well-known/oauth-protected-resource/mcp" in response.headers["www-authenticate"]
        metadata = (await http.get("/.well-known/oauth-protected-resource/mcp")).json()
        assert metadata["resource"] == resource
        assert metadata["authorization_servers"] == [base]
        discovery = (await http.get("/.well-known/oauth-authorization-server")).json()
        assert discovery["issuer"] == base
        assert discovery["code_challenge_methods_supported"] == ["S256"]
        response = await http.post(discovery["registration_endpoint"], json={
            "client_name": "Synthetic Python SDK client",
            "redirect_uris": ["http://127.0.0.1:45678/callback"],
            "token_endpoint_auth_method": "none",
            "grant_types": ["authorization_code"], "response_types": ["code"],
        })
        assert response.status_code == 201
        client = response.json()["client_id"]
        verifier = "v" * 43
        challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b"=").decode()
        params = {
            "client_id": client, "redirect_uri": "http://127.0.0.1:45678/callback",
            "response_type": "code", "resource": resource, "scope": "mcp:access",
            "code_challenge": challenge, "code_challenge_method": "S256", "state": "synthetic-state",
        }
        response = await http.get(discovery["authorization_endpoint"], params={**params, "scope": "unknown"})
        assert response.status_code == 303
        assert parse_qs(urlparse(response.headers["location"]).query)["error"] == ["invalid_scope"]

        async def consent(decision):
            response = await http.get(discovery["authorization_endpoint"], params=params)
            assert response.status_code == 200
            pending = re.search(r'name="request" value="([^"]+)"', response.text)[1]
            response = await http.post(discovery["authorization_endpoint"], headers={"Origin": base}, data={
                "request": pending, "decision": decision,
                "email": "owner@example.com", "password": "synthetic-password",
            })
            assert response.status_code == 200
            callback = html.unescape(re.search(r'<a href="([^"]+)">', response.text)[1])
            result = parse_qs(urlparse(callback).query)
            assert result["state"] == [params["state"]]
            assert result["iss"] == [base]
            return result

        denied = await consent("deny")
        assert denied["error"] == ["access_denied"] and "code" not in denied
        approved = await consent("allow")
        exchange = {
            "grant_type": "authorization_code", "code": approved["code"][0],
            "client_id": client, "redirect_uri": params["redirect_uri"],
            "resource": resource, "code_verifier": verifier,
        }
        response = await http.post(discovery["token_endpoint"], data=exchange)
        assert response.status_code == 200
        token = response.json()["access_token"]
        assert response.json()["scope"] == "mcp:access"
        assert response.json()["expires_in"] == 3600
        assert (await http.post(discovery["token_endpoint"], data=exchange)).status_code == 400
        async with httpx.AsyncClient(headers={"Authorization": "Bearer " + token}, timeout=10, trust_env=False) as mcp_http, streamable_http_client(resource, http_client=mcp_http) as (read, write, _):
            async with ClientSession(read, write, read_timeout_seconds=timedelta(seconds=10)) as session:
                initialized = await session.initialize()
                assert initialized.serverInfo.name == "julius"
                assert initialized.capabilities.tools is not None
                tools = await session.list_tools()
                assert tools.tools == []  # No financial tools in this milestone.
                try:
                    await session.call_tool("missing", {})
                except McpError as error:
                    assert error.error.code == -32602
                else:
                    raise AssertionError("unknown tool accepted")
                response = await http.post(discovery["revocation_endpoint"], data={"client_id": client, "token": token})
                assert response.status_code == 200
                response = await mcp_http.post(resource, json={"jsonrpc": "2.0", "id": 99, "method": "tools/list"})
                assert response.status_code == 401
                assert "resource_metadata" in response.headers["www-authenticate"]
    print("Python SDK: OAuth discovery/denial/scope/PKCE, MCP discovery/error, revocation passed")


asyncio.run(main())
