# oidc-proxy-workaround
A reverse proxy to modify OIDC requests

This reverse proxy listens for POST requests to /token and forwards the request to a token endpoint of an OAuth server. It modifies the response by copying the returned access_token field to the id_token field.

Created as a special workaround for an application that expects an id_token in the response and an IDP server that returns the JWT with the user information only in the access_token field.

This reverse proxy does not implement https, so use for learning purpose only.