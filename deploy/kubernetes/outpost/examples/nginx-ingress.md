# NGINX Ingress ForwardAuth

NGINX `auth_request` treats only `2xx`, `401`, and `403` as valid auth responses. Use the Outpost `mode=nginx` query parameter so a login redirect becomes `401`; `auth-signin` then starts the Soha Proxy login flow.

```yaml
metadata:
  annotations:
    nginx.ingress.kubernetes.io/auth-url: "http://soha-outpost.soha-outpost.svc.cluster.local/api/v1/outpost/forward-auth?mode=nginx"
    nginx.ingress.kubernetes.io/auth-signin: "https://soha.example.com/api/v1/provider/proxy/start?return_to=$scheme://$http_host$escaped_request_uri"
    nginx.ingress.kubernetes.io/auth-response-headers: "X-Soha-User,X-Soha-User-ID,X-Soha-Email,X-Soha-Roles,X-Soha-Teams,X-Soha-Groups,X-Soha-Projects,X-Soha-Tags"
    nginx.ingress.kubernetes.io/auth-snippet: |
      proxy_set_header X-Soha-Outpost-Token "REPLACE_FROM_SECRET_MANAGER";
      proxy_set_header X-Forwarded-Uri $request_uri;
      proxy_set_header X-Forwarded-Host $host;
      proxy_set_header X-Forwarded-Method $request_method;
```

Render `REPLACE_FROM_SECRET_MANAGER` during deployment with a secret-aware GitOps mechanism. Do not commit the agent token in the Ingress. The browser automatically sends the `soha_proxy_session` cookie to the auth subrequest when the Proxy provider cookie domain covers the protected host.
