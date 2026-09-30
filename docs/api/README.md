# Contrato HTTP

El [contrato OpenAPI](openapi.yaml) describe `/healthz`, `/readyz` y `/v1/me`.
Se ampliará al implementar esquemas, errores, idempotencia y SSE
para estos endpoints pendientes:

- `POST /v1/products`
- `POST /v1/sales`
- `POST /v1/sales/{id}/pay`
- `GET /v1/sales/{id}`
- `GET /v1/dashboard/today`
- `GET /v1/dashboard/stream`
