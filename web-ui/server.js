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

// Validate endpoint
app.post('/api/validate', (req, res) => {
    const { idea, params } = req.body;
    
    if (!idea || !idea.name || !idea.problem || !idea.solution) {
        return res.status(400).json({ error: 'Missing required idea fields' });
    }
    
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
        '--personas', String(params.personas || 100),
        '--min-score', String(params.minScore || 0.5),
        '--profile', PROFILE_PATH,
        '--output', RESULTS_DIR,
        '--verbose'
    ];
    
    console.log('Running:', GO_BINARY, args.join(' '));
    
    // Set headers for streaming
    res.setHeader('Content-Type', 'text/plain');
    res.setHeader('Transfer-Encoding', 'chunked');
    
    // Spawn process
    const proc = spawn(GO_BINARY, args, {
        cwd: path.dirname(GO_BINARY)
    });
    
    let outputBuffer = '';
    
    proc.stdout.on('data', (data) => {
        const text = data.toString();
        outputBuffer += text;
        res.write(text);
        
        // If we see a complete JSON result, send it
        if (text.includes('"Ideas":')) {
            const lines = outputBuffer.split('\n');
            for (const line of lines) {
                if (line.trim().startsWith('{') && line.includes('"Ideas":')) {
                    try {
                        JSON.parse(line); // Validate JSON
                        res.write('\n' + line + '\n');
                    } catch (e) {}
                }
            }
        }
    });
    
    proc.stderr.on('data', (data) => {
        res.write(`[STDERR] ${data.toString()}`);
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
        res.write(`\nERROR: ${err.message}\n`);
        res.end();
    });
});

// History endpoint
app.get('/api/history', (req, res) => {
    try {
        const files = fs.readdirSync(RESULTS_DIR)
            .filter(f => f.endsWith('.json'))
            .map(f => {
                const fullPath = path.join(RESULTS_DIR, f);
                const stats = fs.statSync(fullPath);
                
                // Try to read idea name from file
                let name = 'Validation Run';
                let ideas = 0;
                try {
                    const content = JSON.parse(fs.readFileSync(fullPath, 'utf8'));
                    if (content.Ideas && content.Ideas.length > 0) {
                        name = content.Ideas[0].Idea.Name || name;
                        ideas = content.Ideas.length;
                    }
                } catch (e) {}
                
                return {
                    file: f,
                    name,
                    ideas,
                    timestamp: stats.mtime,
                    size: stats.size
                };
            })
            .sort((a, b) => b.timestamp - a.timestamp);
        
        res.json(files);
    } catch (error) {
        console.error('History error:', error);
        res.status(500).json({ error: error.message });
    }
});

// Load specific run
app.get('/api/run/:filename', (req, res) => {
    try {
        const filename = req.params.filename;
        const filePath = path.join(RESULTS_DIR, filename);
        
        if (!fs.existsSync(filePath)) {
            return res.status(404).json({ error: 'Run not found' });
        }
        
        const content = fs.readFileSync(filePath, 'utf8');
        res.json(JSON.parse(content));
    } catch (error) {
        console.error('Load run error:', error);
        res.status(500).json({ error: error.message });
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
    console.log(`🚀 Startup Factory UI running on http://localhost:${PORT}`);
    console.log(`Binary: ${GO_BINARY}`);
    console.log(`Profile: ${PROFILE_PATH}`);
    console.log(`Results: ${RESULTS_DIR}`);
});
