const express = require('express');
const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');
const cors = require('cors');

const app = express();
const PORT = process.env.PORT || 3737;

app.use(cors());
app.use(express.json());
app.use(express.static(__dirname));

// Paths
const GO_BINARY = path.join(__dirname, '../go-validator/startup-factory');
const PROFILE_PATH = path.join(__dirname, '../profile.json');
const RESULTS_DIR = path.join(__dirname, '../go-validator/results');

// Serve UI
app.get('/', (req, res) => {
    res.sendFile(path.join(__dirname, 'index.html'));
});

// Enhanced validation with SSE streaming - parses Go EVENT: lines
app.post('/api/validate-detailed', (req, res) => {
    const { idea, params } = req.body;
    
    if (!idea || !idea.name || !idea.problem || !idea.solution) {
        return res.status(400).json({ error: 'Missing required idea fields' });
    }
    
    // Setup SSE
    res.setHeader('Content-Type', 'text/event-stream');
    res.setHeader('Cache-Control', 'no-cache');
    res.setHeader('Connection', 'keep-alive');

    const sendEvent = (eventData) => {
        res.write(`data: ${JSON.stringify(eventData)}\n\n`);
    };

    // Create temporary idea file
    const ideaFile = path.join(__dirname, `idea-${Date.now()}.json`);
    const ideaData = {
        name: idea.name,
        problem: idea.problem,
        solution: idea.solution,
        target_audience: idea.target_audience || 'General users',
        pricing_tier: idea.pricing || 29
    };
    
    fs.writeFileSync(ideaFile, JSON.stringify(ideaData, null, 2));
    
    // Build command
    const args = [
        '--mode', 'pipeline',
        '--pinned-idea', ideaFile,
        '--personas', String(params.personas || 20),
        '--min-score', String(params.minScore || 0.5),
        '--profile', PROFILE_PATH,
        '--output', RESULTS_DIR,
        '--verbose'
    ];
    
    console.log('Running:', GO_BINARY, args.join(' '));
    
    // Spawn process
    const proc = spawn(GO_BINARY, args, {
        cwd: path.dirname(GO_BINARY)
    });
    
    proc.stdout.on('data', (data) => {
        const text = data.toString();
        console.log('[STDOUT]', text);
        
        // Parse EVENT: lines
        const lines = text.split('\n');
        for (const line of lines) {
            if (line.startsWith('EVENT: ')) {
                try {
                    const eventData = JSON.parse(line.slice(7));
                    sendEvent(eventData);
                } catch (e) {
                    console.error('Failed to parse event:', line, e);
                }
            }
        }
    });
    
    proc.stderr.on('data', (data) => {
        console.error('[STDERR]', data.toString());
    });
    
    proc.on('close', (code) => {
        console.log(`Validation process exited with code ${code}`);
        
        // Clean up temp file
        try {
            fs.unlinkSync(ideaFile);
        } catch (e) {}
        
        res.end();
    });
    
    proc.on('error', (err) => {
        console.error('Spawn error:', err);
        sendEvent({
            type: 'error',
            payload: { message: err.message }
        });
        res.end();
    });
    
    req.on('close', () => {
        proc.kill();
    });
});

// Serve artifacts (landing pages, personas, results)
app.get('/api/artifacts/:runId/:file', (req, res) => {
    const { runId, file } = req.params;
    const filePath = path.join(RESULTS_DIR, runId, file);
    
    if (!fs.existsSync(filePath)) {
        return res.status(404).json({ error: 'Artifact not found' });
    }
    
    // Serve HTML files directly
    if (file.endsWith('.html')) {
        res.sendFile(filePath);
    } else {
        // JSON files
        const content = fs.readFileSync(filePath, 'utf8');
        res.json(JSON.parse(content));
    }
});

// Health check
app.get('/api/health', (req, res) => {
    res.json({
        status: 'ok',
        binary: fs.existsSync(GO_BINARY),
        profile: fs.existsSync(PROFILE_PATH),
        results_dir: fs.existsSync(RESULTS_DIR)
    });
});

app.listen(PORT, () => {
    console.log(`🚀 Startup Factory Dashboard running on http://localhost:${PORT}`);
    console.log(`Binary: ${GO_BINARY}`);
    console.log(`Profile: ${PROFILE_PATH}`);
    console.log(`Results: ${RESULTS_DIR}`);
});
