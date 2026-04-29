# something-backend
Go Backend for somethingmatters.ca webapp

## Launch campaign endpoint

Send the website launch notification email to a provided list of recipients:

`POST /campaigns/launch`

Required header:

`X-Campaign-API-Key: <CAMPAIGN_API_KEY>`

Required env vars:

- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `SMTP_FROM`
- `APP_BASE_URL`
- `CAMPAIGN_API_KEY`

Request body:

```json
{
  "emails": [
    "first@example.com",
    "second@example.com"
  ]
}
```
