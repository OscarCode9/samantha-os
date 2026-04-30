---
name: samantha-server-deploy
description: "Deploy or update the Samantha landing page at samantha.oventlabs.com on the cecaop EC2 instance, shared ALB, and Hostinger DNS. Use when asked to publish landing-page changes, fix Samantha HTTPS routing, or verify the Samantha/cecaop shared infrastructure without breaking existing domains."
---

# Samantha Server Deploy

Use this skill when the task is to publish or debug `samantha.oventlabs.com`.

This setup is fragile because Samantha shares infrastructure with other domains. Prefer additive changes only.

## Scope

- Local landing source: `/Users/oscarcode/elementary-claw/landing-page`
- SSH key: `/Users/oscarcode/.ssh/cecaop-key.pem`
- EC2 host: `ec2-user@3.130.143.156`
- EC2 instance id: `i-0d83d46ea93fd0c1c`
- Static site directory on server: `/home/ec2-user/learning-platform/samantha-site`
- Docker proxy container: `cecaop-nginx-proxy`
- Nginx config on server: `/home/ec2-user/learning-platform/nginx/nginx.conf`
- Shared ALB DNS: `truck-398833611.us-east-2.elb.amazonaws.com`
- Shared ALB ARN: `arn:aws:elasticloadbalancing:us-east-2:799168220850:loadbalancer/app/truck/8f3eedc40c409e7a`
- HTTPS listener ARN: `arn:aws:elasticloadbalancing:us-east-2:799168220850:listener/app/truck/8f3eedc40c409e7a/2516e1cd01db899c`
- Samantha ACM cert ARN: `arn:aws:acm:us-east-2:799168220850:certificate/7e2c9636-0008-4adc-8bd1-a619bacc07f1`
- Samantha target group ARN: `arn:aws:elasticloadbalancing:us-east-2:799168220850:targetgroup/samantha-landing-80/6551c0dfa355375e`
- Hostinger token file: `/Users/oscarcode/elementary-claw/landing-page/hostinguer.txt`

## Rules

- Do not touch existing host rules for `cecaop.online`, `agents.oventlabs.com`, `tenk.oventlabs.com`, or other domains unless the user explicitly asks.
- Prefer ALB host-header routing over custom HTTPS logic inside the EC2 proxy.
- When changing DNS, update only the `samantha` record in `oventlabs.com`.
- Verify `cecaop.online` still works after infra changes.

## Standard deploy flow

### 1. Build locally

Run from `/Users/oscarcode/elementary-claw/landing-page`:

```bash
npm run build
```

### 2. Upload the static build to the EC2 instance

Use `rsync` and avoid macOS junk files:

```bash
rsync -av --delete \
  --exclude '.DS_Store' \
  --exclude '._*' \
  /Users/oscarcode/elementary-claw/landing-page/dist/ \
  ec2-user@3.130.143.156:/home/ec2-user/learning-platform/samantha-site/
```

If needed, clean stale Apple metadata on the server:

```bash
ssh -i /Users/oscarcode/.ssh/cecaop-key.pem ec2-user@3.130.143.156 \
  "find /home/ec2-user/learning-platform/samantha-site -name '._*' -delete"
```

### 3. Verify the direct EC2 HTTP site

```bash
curl -I http://samantha.oventlabs.com/
curl -s http://samantha.oventlabs.com/ | sed -n '1,20p'
```

If DNS is already behind the ALB and you need to verify the origin directly, use SSH plus file inspection on the server:

```bash
ssh -i /Users/oscarcode/.ssh/cecaop-key.pem ec2-user@3.130.143.156 \
  "sed -n '1,40p' /home/ec2-user/learning-platform/samantha-site/index.html"
```

## HTTPS / ALB repair flow

Use this when `https://samantha.oventlabs.com` redirects to `cecaop.online` or serves the wrong certificate.

### 1. Confirm the current symptoms

```bash
curl -I http://samantha.oventlabs.com/
curl -k -I https://samantha.oventlabs.com/
dig +short samantha.oventlabs.com
```

### 2. Verify ALB pieces

Check that:

- Samantha certificate is attached to the HTTPS listener
- a `host-header` rule exists for `samantha.oventlabs.com`
- target group `samantha-landing-80` is healthy

Useful commands:

```bash
aws elbv2 describe-listener-certificates --region us-east-2 --listener-arn "$HTTPS_LISTENER_ARN"
aws elbv2 describe-rules --region us-east-2 --listener-arn "$HTTPS_LISTENER_ARN"
aws elbv2 describe-target-health --region us-east-2 --target-group-arn "$TG_ARN"
```

### 3. Test ALB before DNS cutover

```bash
curl -I --connect-to samantha.oventlabs.com:443:truck-398833611.us-east-2.elb.amazonaws.com:443 \
  https://samantha.oventlabs.com/
```

Only switch DNS after this returns `200`.

## Hostinger DNS cutover

Read the bearer token from:

```bash
/Users/oscarcode/elementary-claw/landing-page/hostinguer.txt
```

The API base for zone records is:

```bash
https://developers.hostinger.com/api/dns/v1/zones/oventlabs.com
```

### Inspect current Samantha records

```bash
HOSTINGER_TOKEN=$(tail -n 1 /Users/oscarcode/elementary-claw/landing-page/hostinguer.txt)
curl -s -H "Authorization: Bearer $HOSTINGER_TOKEN" \
  https://developers.hostinger.com/api/dns/v1/zones/oventlabs.com | \
  jq '.[] | select(.name=="samantha" or (.name | contains(".samantha")))'
```

### Cut over Samantha to the ALB

Delete the old `A` record first, then create the `CNAME`:

```bash
curl -s -X DELETE \
  -H "Authorization: Bearer $HOSTINGER_TOKEN" \
  -H 'Content-Type: application/json' \
  https://developers.hostinger.com/api/dns/v1/zones/oventlabs.com \
  -d '{"filters":[{"name":"samantha","type":"A"}]}'

curl -s -X PUT \
  -H "Authorization: Bearer $HOSTINGER_TOKEN" \
  -H 'Content-Type: application/json' \
  https://developers.hostinger.com/api/dns/v1/zones/oventlabs.com \
  -d '{"overwrite":true,"zone":[{"name":"samantha","type":"CNAME","ttl":300,"records":[{"content":"truck-398833611.us-east-2.elb.amazonaws.com."}]}]}'
```

## Final verification checklist

Run all of these:

```bash
curl -I http://samantha.oventlabs.com/
curl -I https://samantha.oventlabs.com/
curl -s https://samantha.oventlabs.com/ | sed -n '1,20p'
curl -I https://cecaop.online/
curl -I https://agents.oventlabs.com/
dig +short samantha.oventlabs.com @1.1.1.1
dig +short samantha.oventlabs.com @8.8.8.8
```

Expected:

- `http://samantha.oventlabs.com` returns `301` to HTTPS
- `https://samantha.oventlabs.com` returns `200`
- the HTML body is Samantha’s static landing page
- `cecaop.online` still returns `200`
- `agents.oventlabs.com` still returns `200`

## If HTTPS breaks again

Inspect the reverse proxy config inside the EC2 host:

```bash
ssh -i /Users/oscarcode/.ssh/cecaop-key.pem ec2-user@3.130.143.156 \
  "sed -n '1,260p' /home/ec2-user/learning-platform/nginx/nginx.conf"
```

Inspect containers:

```bash
ssh -i /Users/oscarcode/.ssh/cecaop-key.pem ec2-user@3.130.143.156 \
  "docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'"
```

The Samantha static files are mounted into the proxy from:

```bash
/home/ec2-user/learning-platform/samantha-site
```
