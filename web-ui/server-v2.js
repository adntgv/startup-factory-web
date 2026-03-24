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

// Serve new UI
app.get('/', (req, res) => {
    res.sendFile(path.join(__dirname, 'index-v2.html'));
});

// Enhanced validation with SSE streaming
app.post('/api/validate-detailed', (req, res) => {
    const { idea, params } = req.body;
    
    if (!idea || !idea.name || !idea.problem || !idea.solution) {
        return res.status(400).json({ error: 'Missing required idea fields' });
    }
    
    // Setup SSE
    res.setHeader('Content-Type', 'text/event-stream');
    res.setHeader('Cache-Control', 'no-cache');
    res.setHeader('Connection', 'keep-alive');

    const sendEvent = (type, data) => {
        res.write(`data: ${JSON.stringify({ type, ...data })}\n\n`);
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
    
    // Initial progress
    sendEvent('progress', { percent: 5, message: 'Initializing...' });
    sendEvent('phase', { phase: 'idea', state: 'active', message: 'Analyzing idea...' });
    
    // Spawn process
    const proc = spawn(GO_BINARY, args, {
        cwd: path.dirname(GO_BINARY)
    });
    
    let personas = [];
    let landingPageGenerated = false;
    let currentPhase = 'idea';
    
    proc.stdout.on('data', (data) => {
        const text = data.toString();
        console.log(text);
        
        // Parse output for events
        const lines = text.split('\n');
        for (const line of lines) {
            // Detect phase transitions
            if (line.includes('Generating personas')) {
                sendEvent('progress', { percent: 20, message: 'Generating AI personas...' });
                sendEvent('phase', { phase: 'idea', state: 'complete', message: 'Complete' });
                sendEvent('phase', { phase: 'personas', state: 'active', message: 'Generating personas...' });
                currentPhase = 'personas';
            }
            
            // Mock landing page generation (validator doesn't generate HTML yet)
            if (line.includes('Generating personas') && !landingPageGenerated) {
                landingPageGenerated = true;
                sendEvent('progress', { percent: 15, message: 'Creating landing page...' });
                sendEvent('phase', { phase: 'landing', state: 'active', message: 'Generating...' });
                
                setTimeout(() => {
                    const landingHTML = generateMockLandingPage(idea);
                    sendEvent('landing_page', { html: landingHTML });
                    sendEvent('phase', { phase: 'landing', state: 'complete', message: 'Complete' });
                }, 1000);
            }
            
            // Detect persona generation
            if (line.includes('persona') || line.match(/\d+\/\d+/)) {
                const match = line.match(/(\d+)\/(\d+)/);
                if (match) {
                    const current = parseInt(match[1]);
                    const total = parseInt(match[2]);
                    const percent = 20 + (current / total * 30);
                    sendEvent('progress', { percent, message: `Generated ${current}/${total} personas` });
                    
                    // Send mock persona data
                    if (personas.length < current) {
                        const persona = generateMockPersona(current, idea);
                        personas.push(persona);
                        sendEvent('persona', { persona });
                    }
                }
            }
            
            // Detect validation phase
            if (line.includes('Simulating') || line.includes('validation')) {
                if (currentPhase !== 'validation') {
                    sendEvent('phase', { phase: 'personas', state: 'complete', message: `${personas.length} generated` });
                    sendEvent('phase', { phase: 'validation', state: 'active', message: 'Validating...' });
                    sendEvent('progress', { percent: 60, message: 'Running persona validations...' });
                    currentPhase = 'validation';
                }
                
                // Mock persona validations
                const match = line.match(/(\d+)\/(\d+)/);
                if (match) {
                    const current = parseInt(match[1]);
                    const total = parseInt(match[2]);
                    const percent = 60 + (current / total * 30);
                    sendEvent('progress', { percent, message: `Validated ${current}/${total} personas` });
                    
                    // Send validation result for persona
                    if (personas[current - 1]) {
                        const validation = generateMockValidation(personas[current - 1], idea);
                        sendEvent('persona_validation', { 
                            personaId: personas[current - 1].id, 
                            validation 
                        });
                    }
                }
            }
            
            // Detect completion
            if (line.includes('"Score":') || line.includes('"Ideas":')) {
                try {
                    const jsonMatch = line.match(/\{.*\}/);
                    if (jsonMatch) {
                        const result = JSON.parse(jsonMatch[0]);
                        if (result.Ideas && result.Ideas.length > 0) {
                            const topIdea = result.Ideas[0];
                            sendEvent('progress', { percent: 100, message: 'Validation complete!' });
                            sendEvent('phase', { phase: 'validation', state: 'complete', message: 'Complete' });
                            sendEvent('results', {
                                data: {
                                    score: topIdea.Score || 0,
                                    conversion_rate: topIdea.ConversionRate || 0,
                                    hope_met: topIdea.HopeMet || 0,
                                    hope_gap_reason: topIdea.HopeGapReason || null,
                                    summary: `Validated with ${personas.length} personas. ${Math.round((topIdea.ConversionRate || 0) * 100)}% conversion rate.`
                                }
                            });
                        }
                    }
                } catch (e) {}
            }
        }
    });
    
    proc.stderr.on('data', (data) => {
        console.error('STDERR:', data.toString());
    });
    
    proc.on('close', (code) => {
        console.log(`Process exited with code ${code}`);
        
        // Clean up
        try {
            fs.unlinkSync(ideaFile);
        } catch (e) {}
        
        res.end();
    });
    
    req.on('close', () => {
        proc.kill();
    });
});

// Helper: Generate mock landing page HTML
function generateMockLandingPage(idea) {
    return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>${idea.name}</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { 
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            color: #333;
        }
        .hero { 
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 80px 20px;
            text-align: center;
        }
        .hero h1 { font-size: 48px; margin-bottom: 20px; }
        .hero p { font-size: 20px; margin-bottom: 30px; opacity: 0.9; }
        .cta { 
            background: white;
            color: #667eea;
            padding: 15px 40px;
            border-radius: 8px;
            font-size: 18px;
            font-weight: 600;
            border: none;
            cursor: pointer;
            display: inline-block;
            text-decoration: none;
        }
        .features {
            max-width: 1200px;
            margin: 80px auto;
            padding: 0 20px;
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 40px;
        }
        .feature {
            text-align: center;
            padding: 30px;
        }
        .feature-icon { font-size: 48px; margin-bottom: 20px; }
        .feature h3 { font-size: 24px; margin-bottom: 15px; color: #667eea; }
        .pricing {
            background: #f7f7f7;
            padding: 80px 20px;
            text-align: center;
        }
        .pricing h2 { font-size: 36px; margin-bottom: 20px; }
        .price { font-size: 48px; font-weight: 700; color: #667eea; margin: 20px 0; }
    </style>
</head>
<body>
    <div class="hero">
        <h1>${idea.name}</h1>
        <p>${idea.problem}</p>
        <a href="#" class="cta">Get Started</a>
    </div>
    
    <div class="features">
        <div class="feature">
            <div class="feature-icon">⚡</div>
            <h3>Fast & Easy</h3>
            <p>Get started in minutes, not hours</p>
        </div>
        <div class="feature">
            <div class="feature-icon">🎯</div>
            <h3>Precise Solution</h3>
            <p>${idea.solution}</p>
        </div>
        <div class="feature">
            <div class="feature-icon">💪</div>
            <h3>Powerful Results</h3>
            <p>Achieve your goals faster</p>
        </div>
    </div>
    
    <div class="pricing">
        <h2>Simple, Transparent Pricing</h2>
        <div class="price">$${idea.pricing}<span style="font-size: 24px; color: #666;">/month</span></div>
        <a href="#" class="cta">Start Free Trial</a>
    </div>
</body>
</html>
    `;
}

// Helper: Generate mock persona
function generateMockPersona(id, idea) {
    const roles = [
        'Startup Founder', 'Product Manager', 'Marketing Director', 'CTO', 'Sales Lead',
        'Designer', 'Developer', 'CEO', 'Entrepreneur', 'Consultant'
    ];
    const companies = ['tech startup', 'SaaS company', 'agency', 'enterprise', 'e-commerce'];
    const backgrounds = [
        'Has struggled with this problem for years',
        'Recently started looking for solutions',
        'Currently using a competitor',
        'Built internal tools to solve this',
        'Evaluating multiple solutions'
    ];
    
    return {
        id,
        name: `Persona ${id}`,
        avatar: ['👨‍💼', '👩‍💼', '🧑‍💻', '👨‍🔬', '👩‍🎨'][id % 5],
        role: roles[id % roles.length],
        company: companies[id % companies.length],
        background: backgrounds[id % backgrounds.length],
        hope: `Hopes to solve ${idea.problem.toLowerCase()} efficiently`
    };
}

// Helper: Generate mock validation
function generateMockValidation(persona, idea) {
    const score = 0.3 + Math.random() * 0.7;
    const decision = score > 0.5 ? 'convert' : 'pass';
    const hopeMet = score > 0.6;
    
    return {
        decision,
        score,
        hope_met: hopeMet,
        reasoning: decision === 'convert' 
            ? `This solution directly addresses my needs. ${idea.solution} seems like exactly what I've been looking for.`
            : `While this is interesting, it doesn't fully solve my specific problem. I would need more features.`
    };
}

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
