# Deployment Instructions

## Manual Deployment to Coolify

1. **Log in to Coolify:** http://localhost:8000
2. **Create New Resource** → **Application** → **Public Repository**
3. **Configuration:**
   - Repository: `https://github.com/adntgv/startup-factory`
   - Branch: `main`
   - Build Pack: `Dockerfile`
   - Dockerfile Location: `web-ui/Dockerfile`
   - Port: `3737`
   - Domain: `factory.adntgv.com`
4. **Deploy**

## Environment Variables

No environment variables needed initially. The app uses MaxClaw at maxclaw:9999.

## Local Testing

```bash
cd ~/workspace/startup-factory/web-ui
node server.js
```

Then visit http://localhost:3737
