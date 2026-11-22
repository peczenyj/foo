# Black-box checks against a running server: scripts/hurl.sh [base-url].
# JSON only on success; errors are plain status codes.

GET {{base_url}}/foo
HTTP 200
[Asserts]
header "Content-Type" == "application/json"
jsonpath "$.message" == "bar"

GET {{base_url}}/bar
HTTP 404

POST {{base_url}}/foo
HTTP 405
[Asserts]
header "Allow" == "GET, HEAD"

GET {{base_url}}/healthz
HTTP 200
