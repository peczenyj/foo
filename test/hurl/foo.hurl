# Black-box checks against a running server: scripts/hurl.sh [base-url].
# GET /foo answers JSON; its errors are RFC 9457 problem details
# (application/problem+json). The real Fooer never fails, so the problem
# responses are covered by the Go tests; here, the router's own 404 and 405
# keep net/http's plain text.

GET {{base_url}}/foo
HTTP 200
[Asserts]
header "Content-Type" == "application/json"
jsonpath "$.message" == "bar"

# HEAD is served by the GET route: the same headers, no body.
HEAD {{base_url}}/foo
HTTP 200
[Asserts]
header "Content-Type" == "application/json"
bytes count == 0

GET {{base_url}}/bar
HTTP 404
[Asserts]
header "Content-Type" == "text/plain; charset=utf-8"

POST {{base_url}}/foo
HTTP 405
[Asserts]
header "Allow" == "GET, HEAD"
header "Content-Type" == "text/plain; charset=utf-8"

GET {{base_url}}/healthz
HTTP 200
