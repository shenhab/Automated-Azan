# Deploying Automated Azan on Portainer

This guide will help you deploy the Automated Azan application on Portainer using Docker Compose stacks.

## Prerequisites

1. **Portainer installed and running** on your server
2. **Docker and Docker Compose** installed on the host
3. **Network requirements**: The host must be on the same network as your Chromecast devices
4. **Configuration file**: You need to have your `adahn.config` file ready

## Deployment Options

### Option 1: Using Portainer Stacks (Recommended)

1. **Access Portainer UI**
   - Open your browser and navigate to your Portainer instance
   - Log in with your credentials

2. **Create a New Stack**
   - Go to "Stacks" in the left menu
   - Click "Add stack"
   - Give it a name: `automated-azan`

3. **Upload the Stack Configuration**
   - Choose "Upload" method
   - Upload the `portainer-stack.yml` file from this repository
   - Or copy-paste the contents from `portainer-stack.yml`

4. **Configure Environment Variables** (Optional)
   - Scroll down to "Environment variables"
   - Add timezone if different from default:
     ```
     TZ=your_timezone_here
     ```

5. **Deploy the Stack**
   - Click "Deploy the stack"
   - Wait for the deployment to complete

### Option 2: Using Portainer App Templates

1. **Create App Template** (Admin only)
   - Go to "App Templates" in the left menu
   - Click "Add template"
   - Use the configuration below:

```json
{
  "type": 2,
  "title": "Automated Azan",
  "description": "Islamic prayer time automation with Chromecast support and web interface",
  "categories": ["IoT", "Automation"],
  "platform": "linux",
  "logo": "https://raw.githubusercontent.com/portainer/portainer/develop/app/assets/images/logos/registry.png",
  "repository": {
    "url": "https://github.com/shenhab/Automated-Azan",
    "stackfile": "portainer-stack.yml"
  },
  "env": [
    {
      "name": "TWILIO_ACCOUNT_SID",
      "label": "Twilio Account SID (Optional)",
      "description": "Your Twilio Account SID for WhatsApp notifications"
    },
    {
      "name": "TWILIO_AUTH_TOKEN",
      "label": "Twilio Auth Token (Optional)",
      "description": "Your Twilio Auth Token"
    },
    {
      "name": "TWILIO_CONTENT_SID",
      "label": "Twilio Content SID (Optional)",
      "description": "Your Twilio Content SID for WhatsApp messages"
    }
  ]
}
```

### Option 3: Manual Container Deployment

If you prefer to deploy containers individually:

1. **Deploy Main Application Container**
   ```bash
   docker run -d \
     --name athan \
     --network host \
     --restart unless-stopped \
     -v azan_logs:/var/log \
     -v azan_config:/app/config \
     -v $(pwd)/adahn.config:/app/adahn.config:ro \
     -e TZ=Europe/Dublin \
     shenhab/athan:latest
   ```

2. **Deploy Web Interface Container**
   ```bash
   docker run -d \
     --name athan-web \
     --restart unless-stopped \
     -p 5000:5000 \
     -v azan_logs:/var/log \
     -v azan_config:/app/config \
     -v $(pwd)/adahn.config:/app/adahn.config:ro \
     -e TZ=Europe/Dublin \
     shenhab/athan-web:latest
   ```

## Configuration

### 1. Configuration File Setup

Before deployment, ensure your `adahn.config` file is properly configured:

```ini
[Settings]
speakers-group-name = athan
location = icci
```

### 2. Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `TZ` | Timezone (e.g., Europe/Dublin) | Yes |
| `LOG_FILE` | Log file path | No (default: /var/log/azan_service.log) |

### 3. Network Configuration

**Important**: The main application container must use `host` networking mode to discover Chromecast devices on your local network. This is automatically configured in the stack file.

## Post-Deployment

### 1. Verify Deployment

1. **Check Container Status**
   - Go to "Containers" in Portainer
   - Verify both containers are running:
     - `automated-azan-main`
     - `automated-azan-web`

2. **Check Logs**
   - Click on each container
   - Go to "Logs" tab
   - Look for successful startup messages

3. **Access Web Interface**
   - Open your browser
   - Navigate to `http://your-server-ip:5000`
   - You should see the Automated Azan dashboard

### 2. Test Chromecast Discovery

1. Go to the "Chromecast Devices" page in the web interface
2. Click "Discover Devices"
3. Verify your Chromecast devices are found

### 3. Monitor Prayer Times

1. Check the "Dashboard" for current prayer times
2. Verify the "Next Prayer" information is correct
3. Monitor the "Logs" page for any issues

## Troubleshooting

### Common Issues

1. **No Chromecast devices found**
   - Ensure the container is using host networking
   - Check that Chromecast devices are on the same network
   - Verify multicast/mDNS is enabled on your network

2. **Web interface not accessible**
   - Check port 5000 is not blocked by firewall
   - Verify the web container is running
   - Check container logs for errors

3. **Prayer times not loading**
   - Check internet connectivity from containers
   - Verify the prayer times API is accessible
   - Check application logs for API errors

### Useful Portainer Features

1. **Container Console Access**
   - Go to container details
   - Click "Console" to access shell
   - Useful for debugging

2. **Volume Management**
   - Go to "Volumes" to manage persistent data
   - `azan_logs`: Contains application logs
   - `azan_config`: Contains configuration files

3. **Stack Updates**
   - Go to your stack
   - Click "Editor" to modify configuration
   - Click "Update the stack" to apply changes

## Backup and Maintenance

### Backup Configuration

1. **Export Stack Configuration**
   - Go to your stack in Portainer
   - Copy the YAML configuration
   - Save it to a file for backup

2. **Backup Volumes**
   ```bash
   docker run --rm -v azan_logs:/source -v $(pwd):/backup alpine tar czf /backup/azan_logs_backup.tar.gz -C /source .
   docker run --rm -v azan_config:/source -v $(pwd):/backup alpine tar czf /backup/azan_config_backup.tar.gz -C /source .
   ```

### Updates

1. **Update Application**
   - Pull latest code from repository
   - Go to your stack in Portainer
   - Click "Update the stack"
   - Enable "Re-pull and redeploy"

2. **Monitor After Updates**
   - Check container logs
   - Verify web interface functionality
   - Test Chromecast connectivity

## Security Considerations

1. **Network Security**
   - Consider using a reverse proxy (Nginx, Traefik)
   - Enable HTTPS for the web interface
   - Restrict access to management ports

2. **Environment Variables**
   - Use Portainer secrets for sensitive data
   - Avoid hardcoding credentials in stack files

3. **Container Security**
   - Regularly update base images
   - Monitor for security vulnerabilities
   - Use read-only mounts where possible

## Support

For issues specific to:
- **Application functionality**: Check the main repository issues
- **Portainer deployment**: Check container logs and Portainer documentation
- **Network connectivity**: Verify multicast/mDNS settings on your network
