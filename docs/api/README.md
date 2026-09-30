# Contrato HTTP

El [contrato OpenAPI](openapi.yaml) describe `/healthz`, `/readyz`, `/v1/me` y
productos y ventas con idempotencia.
Se ampliará al implementar esquemas, errores, idempotencia y SSE
para estos endpoints pendientes:

- `POST /v1/sales/{id}/pay`
- `GET /v1/dashboard/today`
- `GET /v1/dashboard/stream`
