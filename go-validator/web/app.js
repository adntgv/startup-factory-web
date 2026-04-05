// ─────────────────────────────────────────────────────────────────────────────
// State
// ─────────────────────────────────────────────────────────────────────────────
const state = {
  currentUser: null,    // {user_id, email}
  currentRun: null,     // full run object
  currentStep: 1,       // 1-4
  personas: [],         // accumulated from SSE
  validations: {},      // {personaId: validation}
  eventSource: null,    // active SSE connection
};

// ─────────────────────────────────────────────────────────────────────────────
// Canvas section definitions
// ─────────────────────────────────────────────────────────────────────────────
const canvasSections = [
  { key: 'problem',               label: 'Problems',              type: 'list', hint: 'One per line' },
  { key: 'existing_alternatives', label: 'Existing Alternatives', type: 'list', hint: 'How people solve this today' },
  { key: 'solution',              label: 'Solution',              type: 'list', hint: 'Key features, one per line' },
  { key: 'unique_value_prop',     label: 'Unique Value Prop',     type: 'text', hint: 'Clear differentiator' },
  { key: 'high_level_concept',    label: 'High-Level Concept',    type: 'text', hint: 'X for Y analogy' },
  { key: 'unfair_advantage',      label: 'Unfair Advantage',      type: 'text', hint: "Can't be copied or bought" },
  { key: 'customer_segments',     label: 'Customer Segments',     type: 'list', hint: 'Target ICPs, one per line' },
  { key: 'early_adopters',        label: 'Early Adopters',        type: 'list', hint: 'Who buys first and why' },
  { key: 'channels',              label: 'Distribution Channels', type: 'list', hint: 'How you reach customers' },
  { key: 'key_metrics',           label: 'Key Metrics',           type: 'list', hint: 'What you measure' },
  { key: 'cost_structure',        label: 'Cost Structure',        type: 'list', hint: 'Main costs' },
  { key: 'revenue_streams',       label: 'Revenue Streams',       type: 'list', hint: 'How you make money' },
];
// ─────────────────────────────────────────────────────────────────────────────
// Cycling status messages for long LLM operations
// ─────────────────────────────────────────────────────────────────────────────
function startCycling(btnId, messages, ms) {
  ms = ms || 3500;
  var i = 0;
  var spinner = '<span class="spinner"></span>';
  var tick = function() {
    var el = document.getElementById(btnId);
    if (!el) { clearInterval(id); return; }
    el.innerHTML = spinner + '<span>' + messages[i % messages.length] + '</span>';
    i++;
  };
  tick();
  var id = setInterval(tick, ms);
  return id;
}
function stopCycling(id) { if (id != null) clearInterval(id); }


// ─────────────────────────────────────────────────────────────────────────────
// API helpers
// ─────────────────────────────────────────────────────────────────────────────
async function api(method, path, body) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
  };
  if (body !== undefined) {
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  if (res.status === 401) {
    window.location.hash = '#/login';
    throw new Error('Unauthenticated');
  }
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const j = await res.json();
      msg = j.error || j.message || msg;
    } catch (_) {}
    throw new Error(msg);
  }
  if (res.status === 204) return null;
  return res.json();
}

// ─────────────────────────────────────────────────────────────────────────────
// Toast notifications
// ─────────────────────────────────────────────────────────────────────────────
function showToast(message, type = 'error') {
  const container = document.getElementById('toast-container');
  const toast = document.createElement('div');
  const colors = {
    error:   'bg-red-50 border-red-200 text-red-800',
    success: 'bg-green-50 border-green-200 text-green-800',
    info:    'bg-blue-50 border-blue-200 text-blue-800',
  };
  toast.className = `toast border rounded-xl px-4 py-3 shadow-lg text-sm font-medium ${colors[type] || colors.error}`;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transition = 'opacity 0.3s';
    setTimeout(() => toast.remove(), 350);
  }, 4000);
}

// ─────────────────────────────────────────────────────────────────────────────
// Router
// ─────────────────────────────────────────────────────────────────────────────
window.addEventListener('hashchange', route);
window.addEventListener('load', route);

async function route() {
  const hash = window.location.hash || '#/';

  // Always check auth first (except on login page)
  if (hash !== '#/login') {
    if (!state.currentUser) {
      try {
        state.currentUser = await api('GET', '/api/auth/me');
      } catch (_) {
        window.location.hash = '#/login';
        return;
      }
    }
  }

  // Close any open SSE connection when navigating away from step 4
  if (state.eventSource && !hash.startsWith('#/runs/')) {
    state.eventSource.close();
    state.eventSource = null;
  }

  if (hash === '#/login') {
    // If already logged in, redirect home
    if (!state.currentUser) {
      try {
        state.currentUser = await api('GET', '/api/auth/me');
        window.location.hash = '#/';
        return;
      } catch (_) {}
    } else {
      window.location.hash = '#/';
      return;
    }
    renderLogin();
    return;
  }

  if (hash === '#/' || hash === '') {
    renderHome();
    return;
  }

  const runMatch = hash.match(/^#\/runs\/(\d+)$/);
  if (runMatch) {
    renderRun(parseInt(runMatch[1], 10));
    return;
  }

  // Fallback
  renderHome();
}

// ─────────────────────────────────────────────────────────────────────────────
// Utility: set #app innerHTML
// ─────────────────────────────────────────────────────────────────────────────
function setApp(html) {
  document.getElementById('app').innerHTML = html;
}

// ─────────────────────────────────────────────────────────────────────────────
// Screen 1: Login / Register
// ─────────────────────────────────────────────────────────────────────────────
function renderLogin() {
  setApp(`
    <div class="min-h-screen bg-slate-50 flex items-center justify-center px-4">
      <div class="w-full max-w-md">
        <div class="text-center mb-8">
          <div class="inline-flex items-center justify-center w-16 h-16 bg-purple-600 rounded-2xl mb-4 shadow-lg">
            <svg class="w-9 h-9 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.347.284A3.001 3.001 0 0112 21a3.001 3.001 0 01-2.39-1.179l-.346-.283z"/>
            </svg>
          </div>
          <h1 class="text-3xl font-bold text-gray-900">Startup Factory</h1>
          <p class="text-gray-500 mt-1">Validate your startup idea in minutes</p>
        </div>

        <div class="bg-white rounded-2xl shadow-md p-8">
          <!-- Tab toggle -->
          <div class="flex rounded-xl bg-gray-100 p-1 mb-6" id="auth-tabs">
            <button id="tab-login" onclick="switchAuthTab('login')"
              class="flex-1 py-2 rounded-lg text-sm font-semibold transition bg-white shadow text-purple-700">
              Sign In
            </button>
            <button id="tab-register" onclick="switchAuthTab('register')"
              class="flex-1 py-2 rounded-lg text-sm font-semibold transition text-gray-500">
              Create Account
            </button>
          </div>

          <form id="auth-form" onsubmit="submitAuth(event)">
            <div class="space-y-4">
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-1">Email</label>
                <input type="email" id="auth-email" required autocomplete="email"
                  placeholder="you@example.com"
                  class="w-full px-4 py-2.5 border border-gray-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent text-sm"/>
              </div>
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-1">Password</label>
                <input type="password" id="auth-password" required autocomplete="current-password"
                  placeholder="••••••••"
                  class="w-full px-4 py-2.5 border border-gray-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent text-sm"/>
              </div>
            </div>

            <button type="submit" id="auth-submit"
              class="w-full mt-6 py-3 bg-purple-600 hover:bg-purple-700 text-white font-semibold rounded-xl transition flex items-center justify-center gap-2">
              <span id="auth-btn-text">Sign In</span>
            </button>
          </form>
        </div>

        <p class="text-center text-xs text-gray-400 mt-6">
          Validate ideas. Build faster. Ship smarter.
        </p>
      </div>
    </div>
  `);
}

let authMode = 'login';

function switchAuthTab(mode) {
  authMode = mode;
  const loginTab    = document.getElementById('tab-login');
  const registerTab = document.getElementById('tab-register');
  const btnText     = document.getElementById('auth-btn-text');
  const pwInput     = document.getElementById('auth-password');

  if (mode === 'login') {
    loginTab.className    = 'flex-1 py-2 rounded-lg text-sm font-semibold transition bg-white shadow text-purple-700';
    registerTab.className = 'flex-1 py-2 rounded-lg text-sm font-semibold transition text-gray-500';
    btnText.textContent   = 'Sign In';
    pwInput.autocomplete  = 'current-password';
  } else {
    registerTab.className = 'flex-1 py-2 rounded-lg text-sm font-semibold transition bg-white shadow text-purple-700';
    loginTab.className    = 'flex-1 py-2 rounded-lg text-sm font-semibold transition text-gray-500';
    btnText.textContent   = 'Create Account';
    pwInput.autocomplete  = 'new-password';
  }
}

async function submitAuth(e) {
  e.preventDefault();
  const email    = document.getElementById('auth-email').value.trim();
  const password = document.getElementById('auth-password').value;
  const btn      = document.getElementById('auth-submit');
  const btnText  = document.getElementById('auth-btn-text');

  btn.disabled   = true;
  btnText.innerHTML = '<span class="spinner"></span>';

  try {
    const endpoint = authMode === 'login' ? '/api/auth/login' : '/api/auth/register';
    await api('POST', endpoint, { email, password });
    state.currentUser = await api('GET', '/api/auth/me');
    window.location.hash = '#/';
  } catch (err) {
    showToast(err.message || 'Authentication failed');
    btn.disabled  = false;
    btnText.textContent = authMode === 'login' ? 'Sign In' : 'Create Account';
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Screen 2: Home (Runs List)
// ─────────────────────────────────────────────────────────────────────────────
async function renderHome() {
  setApp(`
    <div class="min-h-screen bg-slate-50">
      ${renderHeader()}
      <main class="max-w-4xl mx-auto px-4 py-8">
        <div class="flex items-center justify-between mb-6">
          <h2 class="text-2xl font-bold text-gray-900">Your Ideas</h2>
          <button onclick="openNewIdeaModal()"
            class="inline-flex items-center gap-2 px-5 py-2.5 bg-purple-600 hover:bg-purple-700 text-white font-semibold rounded-xl transition shadow-sm">
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/>
            </svg>
            New Idea
          </button>
        </div>
        <div id="runs-list">
          <div class="flex justify-center py-12">
            <span class="spinner-lg"></span>
          </div>
        </div>
      </main>
    </div>
  `);

  try {
    const runs = await api('GET', '/api/runs');
    renderRunsList(runs);
  } catch (err) {
    showToast(err.message);
    document.getElementById('runs-list').innerHTML = `
      <div class="text-center py-12 text-gray-400">
        <p>Failed to load runs. <button onclick="renderHome()" class="text-purple-600 hover:underline">Retry</button></p>
      </div>`;
  }
}

function renderRunsList(runs) {
  const container = document.getElementById('runs-list');
  if (!runs || runs.length === 0) {
    container.innerHTML = `
      <div class="text-center py-20">
        <div class="inline-flex items-center justify-center w-16 h-16 bg-purple-100 rounded-full mb-4">
          <svg class="w-8 h-8 text-purple-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.347.284A3.001 3.001 0 0112 21a3.001 3.001 0 01-2.39-1.179l-.346-.283z"/>
          </svg>
        </div>
        <h3 class="text-lg font-semibold text-gray-700 mb-1">No ideas yet</h3>
        <p class="text-gray-400 mb-4">Click "New Idea" to validate your first startup idea</p>
        <button onclick="openNewIdeaModal()"
          class="px-5 py-2.5 bg-purple-600 hover:bg-purple-700 text-white font-semibold rounded-xl transition">
          Get Started
        </button>
      </div>`;
    return;
  }

  container.innerHTML = `
    <div class="space-y-3">
      ${runs.map(r => renderRunCard(r)).join('')}
    </div>`;
}

function renderRunCard(run) {
  const statusConfig = {
    draft:      { label: 'Draft',       cls: 'bg-gray-100 text-gray-600' },
    processing: { label: 'Processing',  cls: 'bg-yellow-100 text-yellow-700' },
    canvas:     { label: 'Canvas Ready',cls: 'bg-blue-100 text-blue-700' },
    landing:    { label: 'Landing Done',cls: 'bg-indigo-100 text-indigo-700' },
    complete:   { label: 'Complete',    cls: 'bg-green-100 text-green-700' },
    done:       { label: 'Complete',    cls: 'bg-green-100 text-green-700' },
  };
  const s   = statusConfig[run.status] || { label: run.status, cls: 'bg-gray-100 text-gray-600' };
  const date = new Date(run.created_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  const idea = run.idea_text || '';
  const truncated = idea.length > 120 ? idea.slice(0, 117) + '…' : idea;

  return `
    <div class="bg-white rounded-xl shadow-md p-5 cursor-pointer hover:shadow-lg transition fade-in"
      onclick="window.location.hash='#/runs/${run.id}'">
      <div class="flex items-start justify-between gap-4">
        <div class="flex-1 min-w-0">
          <p class="text-gray-800 font-medium text-sm leading-relaxed">${escapeHtml(truncated)}</p>
          <p class="text-gray-400 text-xs mt-2">${date}</p>
        </div>
        <span class="shrink-0 inline-block px-2.5 py-1 rounded-full text-xs font-semibold ${s.cls}">${s.label}</span>
      </div>
    </div>`;
}

function openNewIdeaModal() {
  const backdrop = document.createElement('div');
  backdrop.className = 'modal-backdrop fade-in';
  backdrop.id = 'new-idea-modal';
  backdrop.innerHTML = `
    <div class="bg-white rounded-2xl shadow-xl w-full max-w-lg p-8">
      <h3 class="text-xl font-bold text-gray-900 mb-2">Describe Your Idea</h3>
      <p class="text-gray-500 text-sm mb-4">Be specific — who it's for, what problem it solves, and how it works.</p>
      <textarea id="new-idea-text" rows="5" placeholder="e.g. An AI-powered tool that automatically generates and A/B tests landing pages for B2B SaaS companies, reducing the time from idea to validated landing page from weeks to hours..."
        class="w-full px-4 py-3 border border-gray-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-purple-500 text-sm resize-none"></textarea>
      <div id="new-idea-error" class="text-red-600 text-sm mt-2 hidden"></div>
      <div class="flex gap-3 mt-5">
        <button onclick="closeNewIdeaModal()"
          class="flex-1 py-2.5 border border-gray-300 text-gray-700 font-semibold rounded-xl hover:bg-gray-50 transition">
          Cancel
        </button>
        <button onclick="submitNewIdea()"
          class="flex-1 py-2.5 bg-purple-600 hover:bg-purple-700 text-white font-semibold rounded-xl transition flex items-center justify-center gap-2"
          id="new-idea-submit">
          <span>Generate Canvas</span>
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6"/>
          </svg>
        </button>
      </div>
    </div>`;

  backdrop.addEventListener('click', e => { if (e.target === backdrop) closeNewIdeaModal(); });
  document.body.appendChild(backdrop);
  document.getElementById('new-idea-text').focus();
}

function closeNewIdeaModal() {
  const m = document.getElementById('new-idea-modal');
  if (m) m.remove();
}

async function submitNewIdea() {
  const text = (document.getElementById('new-idea-text').value || '').trim();
  if (!text) {
    const errEl = document.getElementById('new-idea-error');
    errEl.textContent = 'Please describe your idea first.';
    errEl.classList.remove('hidden');
    return;
  }

  const btn     = document.getElementById('new-idea-submit');
  btn.disabled  = true;
  const _t1 = startCycling('new-idea-submit', [
    'Analyzing your idea…', 'Building lean canvas…', 'Identifying customer segments…',
    'Mapping revenue streams…', 'Almost done…'
  ]);

  try {
    const result = await api('POST', '/api/runs', { idea_text: text });
    stopCycling(_t1);
    closeNewIdeaModal();
    state.currentStep = 2;
    window.location.hash = `#/runs/${result.run_id}`;
  } catch (err) {
    stopCycling(_t1);
    showToast(err.message || 'Failed to create run');
    btn.disabled = false;
    btn.innerHTML = '<span>Generate Canvas</span><svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6"/></svg>';
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Header
// ─────────────────────────────────────────────────────────────────────────────
function renderHeader() {
  const email = state.currentUser ? state.currentUser.email : '';
  return `
    <header class="bg-white border-b border-gray-200 sticky top-0 z-30">
      <div class="max-w-4xl mx-auto px-4 h-14 flex items-center justify-between">
        <a href="#/" class="flex items-center gap-2 font-bold text-gray-900 hover:text-purple-700 transition">
          <div class="w-7 h-7 bg-purple-600 rounded-lg flex items-center justify-center">
            <svg class="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.347.284A3.001 3.001 0 0112 21a3.001 3.001 0 01-2.39-1.179l-.346-.283z"/>
            </svg>
          </div>
          Startup Factory
        </a>
        <div class="flex items-center gap-3">
          <span class="text-sm text-gray-500 hidden sm:block">${escapeHtml(email)}</span>
          <button onclick="logout()"
            class="text-sm text-gray-600 hover:text-red-600 transition font-medium">
            Logout
          </button>
        </div>
      </div>
    </header>`;
}

async function logout() {
  try {
    await api('POST', '/api/auth/logout', {});
  } catch (_) {}
  state.currentUser = null;
  state.currentRun  = null;
  window.location.hash = '#/login';
}

// ─────────────────────────────────────────────────────────────────────────────
// Screen 3: Run Wizard
// ─────────────────────────────────────────────────────────────────────────────
async function renderRun(id) {
  setApp(`
    <div class="min-h-screen bg-slate-50">
      ${renderHeader()}
      <main class="max-w-4xl mx-auto px-4 py-8">
        <div class="flex justify-center py-12">
          <span class="spinner-lg"></span>
        </div>
      </main>
    </div>`);

  try {
    const run = await api('GET', `/api/runs/${id}`);
    state.currentRun = run;

    // Determine step based on status
    if (state.currentStep === 1) {
      state.currentStep = inferStep(run);
    }

    renderWizard(run);
  } catch (err) {
    showToast(err.message);
    setApp(`
      <div class="min-h-screen bg-slate-50">
        ${renderHeader()}
        <main class="max-w-4xl mx-auto px-4 py-8 text-center">
          <p class="text-gray-500">Failed to load run. <a href="#/" class="text-purple-600 hover:underline">Go home</a></p>
        </main>
      </div>`);
  }
}

function inferStep(run) {
  if (!run) return 1;
  if (run.status === 'draft' || !run.canvas) return 1;
  if (!run.landing) return 2;
  if (run.status === 'done' || run.results) return 4;
  return 3;
}

function renderWizard(run) {
  const step = state.currentStep;
  setApp(`
    <div class="min-h-screen bg-slate-50">
      ${renderHeader()}
      <main class="max-w-4xl mx-auto px-4 py-8">
        <div class="mb-2">
          <a href="#/" class="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-purple-600 transition mb-4">
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"/>
            </svg>
            All Ideas
          </a>
        </div>
        ${renderStepIndicator(step, run)}
        <div id="step-content" class="mt-6">
          ${renderStepContent(step, run)}
        </div>
      </main>
    </div>`);

  // Post-render hooks
  if (step === 4 && run.status !== 'complete') {
    startSimulation(run.id);
  } else if (step === 4 && run.results) {
    // Already complete — show cached results
    state.personas    = run.personas || [];
    state.validations = {};
    // DB stores []SimulationResult; map by index (1-based) to match persona ids
    (run.validations || []).forEach((v, i) => {
      const id = i + 1;
      state.validations[id] = {
        decision:          v.converted ? 'convert' : 'reject',
        converted:         v.converted,
        intent_strength:   v.intent_strength,
        impression_score:  v.impression_score,
        relevance_score:   v.relevance_score,
        pricing_reaction:  v.pricing_reaction,
        friction_points:   v.friction_points,
        reasoning:         v.reasoning,
        decision_timeline: v.decision_timeline,
      };
    });
    renderCachedSimulation(run);
  }
}

function renderStepIndicator(currentStep, run) {
  const steps = [
    { n: 1, label: 'Idea' },
    { n: 2, label: 'Canvas' },
    { n: 3, label: 'Landing' },
    { n: 4, label: 'Validate' },
  ];

  // Determine which steps are accessible
  const maxStep = inferMaxStep(run);

  const items = steps.map((s, idx) => {
    let circleCls, textCls;
    if (s.n < currentStep) {
      circleCls = 'bg-green-500 text-white';
      textCls   = 'text-green-600 font-semibold';
    } else if (s.n === currentStep) {
      circleCls = 'bg-purple-600 text-white';
      textCls   = 'text-purple-700 font-semibold';
    } else {
      circleCls = 'bg-gray-200 text-gray-500';
      textCls   = 'text-gray-400';
    }

    const clickable  = s.n <= maxStep;
    const clickAttr  = clickable ? `onclick="goToStep(${s.n})" style="cursor:pointer"` : '';

    const connector = idx < steps.length - 1
      ? `<div class="step-connector ${s.n < currentStep ? 'done' : ''}"></div>`
      : '';

    return `
      <div class="flex items-center">
        <div class="flex flex-col items-center" ${clickAttr}>
          <div class="w-8 h-8 rounded-full flex items-center justify-center text-sm font-bold transition ${circleCls}">
            ${s.n < currentStep ? '✓' : s.n}
          </div>
          <span class="text-xs mt-1 ${textCls} whitespace-nowrap">${s.label}</span>
        </div>
        ${connector}
      </div>`;
  });

  return `
    <div class="bg-white rounded-xl shadow-sm p-4">
      <div class="flex items-start">${items.join('')}</div>
    </div>`;
}

function inferMaxStep(run) {
  if (!run) return 1;
  if (run.status === 'draft' || !run.canvas) return 1;
  if (!run.landing) return 2;
  if (run.status === 'done' || run.results) return 4;
  return 3;
}

function goToStep(n) {
  state.currentStep = n;
  if (state.eventSource && n !== 4) {
    state.eventSource.close();
    state.eventSource = null;
  }
  renderWizard(state.currentRun);
}

function renderStepContent(step, run) {
  switch (step) {
    case 1: return renderStep1(run);
    case 2: return renderStep2(run);
    case 3: return renderStep3(run);
    case 4: return renderStep4(run);
    default: return renderStep1(run);
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 1: Idea
// ─────────────────────────────────────────────────────────────────────────────
function renderStep1(run) {
  const idea = run ? (run.idea_text || '') : '';
  return `
    <div class="bg-white rounded-2xl shadow-md p-8 fade-in">
      <h2 class="text-xl font-bold text-gray-900 mb-1">Your Startup Idea</h2>
      <p class="text-gray-500 text-sm mb-5">Describe your idea in detail. The more context you provide, the better your Lean Canvas will be.</p>
      <textarea id="idea-text" rows="7"
        placeholder="e.g. An AI-powered code review tool that automatically detects security vulnerabilities in pull requests and suggests fixes, targeting mid-sized SaaS companies with 10-50 engineers..."
        class="w-full px-4 py-3 border border-gray-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-purple-500 text-sm resize-none">${escapeHtml(idea)}</textarea>
      <div class="mt-5 flex justify-end">
        <button onclick="generateCanvas()"
          id="gen-canvas-btn"
          class="inline-flex items-center gap-2 px-6 py-3 bg-purple-600 hover:bg-purple-700 text-white font-semibold rounded-xl transition">
          Generate Canvas
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6"/>
          </svg>
        </button>
      </div>
    </div>`;
}

async function generateCanvas() {
  const text = (document.getElementById('idea-text').value || '').trim();
  if (!text) {
    showToast('Please describe your idea first.');
    return;
  }

  const btn    = document.getElementById('gen-canvas-btn');
  btn.disabled = true;
  const _t2 = startCycling('gen-canvas-btn', [
    'Analyzing your idea…', 'Building lean canvas…', 'Identifying customer segments…',
    'Mapping revenue streams…', 'Almost done…'
  ]);

  try {
    let result;
    if (state.currentRun && state.currentRun.id) {
      // Existing run with draft status — we just POST again? No, re-create.
      // Use PATCH canvas if idea changed, or just POST new run.
      result = await api('POST', '/api/runs', { idea_text: text });
    } else {
      result = await api('POST', '/api/runs', { idea_text: text });
    }
    const run = await api('GET', `/api/runs/${result.run_id}`);
    stopCycling(_t2);
    state.currentRun  = run;
    state.currentStep = 2;
    window.location.hash = `#/runs/${run.id}`;
  } catch (err) {
    stopCycling(_t2);
    showToast(err.message || 'Failed to generate canvas');
    btn.disabled = false;
    btn.innerHTML = 'Generate Canvas <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6"/></svg>';
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 2: Canvas Editor
// ─────────────────────────────────────────────────────────────────────────────
function renderStep2(run) {
  const canvas = run.canvas || {};
  const fields = canvasSections.map(sec => {
    const value = canvas[sec.key];
    let input;
    if (sec.type === 'text') {
      input = `<input type="text" id="canvas-${sec.key}" value="${escapeHtml(value || '')}"
        placeholder="${escapeHtml(sec.hint)}"
        class="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-400 text-sm"/>`;
    } else {
      const lines = Array.isArray(value) ? value.join('\n') : (value || '');
      input = `<textarea id="canvas-${sec.key}" rows="5" placeholder="${escapeHtml(sec.hint)}" oninput="this.style.height='auto';this.style.height=this.scrollHeight+'px'"
        class="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-400 text-sm resize-y">${escapeHtml(lines)}</textarea>`;
    }
    return `
      <div class="bg-white rounded-xl border border-gray-200 p-4">
        <label class="block text-xs font-semibold text-purple-700 uppercase tracking-wide mb-1.5">${sec.label}</label>
        ${input}
        <p class="text-xs text-gray-400 mt-1">${sec.hint}</p>
      </div>`;
  }).join('');

  return `
    <div class="fade-in">
      <div class="bg-white rounded-2xl shadow-md p-6 mb-4">
        <h2 class="text-xl font-bold text-gray-900 mb-1">Lean Canvas</h2>
        <p class="text-gray-500 text-sm">Review and edit your auto-generated canvas. Click "Save Edits" after making changes.</p>
      </div>

      <div class="canvas-grid mb-4">${fields}</div>

      <div class="bg-white rounded-2xl shadow-md p-6 flex flex-col sm:flex-row gap-3 justify-between items-center">
        <button onclick="saveCanvas()"
          id="save-canvas-btn"
          class="inline-flex items-center gap-2 px-5 py-2.5 border border-purple-600 text-purple-600 hover:bg-purple-50 font-semibold rounded-xl transition">
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-3m-1 4l-3 3m0 0l-3-3m3 3V4"/>
          </svg>
          Save Edits
        </button>
        <button onclick="generateLanding()"
          id="gen-landing-btn"
          class="inline-flex items-center gap-2 px-6 py-2.5 bg-purple-600 hover:bg-purple-700 text-white font-semibold rounded-xl transition">
          Generate Landing Page
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6"/>
          </svg>
        </button>
      </div>
    </div>`;
}

function readCanvasFromDOM() {
  const canvas = {};
  canvasSections.forEach(sec => {
    const el = document.getElementById(`canvas-${sec.key}`);
    if (!el) return;
    if (sec.type === 'list') {
      canvas[sec.key] = el.value.split('\n').map(s => s.trim()).filter(Boolean);
    } else {
      canvas[sec.key] = el.value.trim();
    }
  });
  return canvas;
}

async function saveCanvas() {
  const btn = document.getElementById('save-canvas-btn');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span><span>Saving…</span>';

  try {
    const canvas = readCanvasFromDOM();
    await api('PATCH', `/api/runs/${state.currentRun.id}/canvas`, { canvas });
    state.currentRun.canvas = canvas;
    showToast('Canvas saved!', 'success');
  } catch (err) {
    showToast(err.message || 'Failed to save canvas');
  } finally {
    btn.disabled = false;
    btn.innerHTML = `
      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-3m-1 4l-3 3m0 0l-3-3m3 3V4"/>
      </svg>
      Save Edits`;
  }
}

async function generateLanding() {
  // Save current canvas edits first
  try {
    const canvas = readCanvasFromDOM();
    await api('PATCH', `/api/runs/${state.currentRun.id}/canvas`, { canvas });
    state.currentRun.canvas = canvas;
  } catch (_) {}

  const btn    = document.getElementById('gen-landing-btn');
  btn.disabled = true;
  const _t3 = startCycling('gen-landing-btn', [
    'Writing headline…', 'Crafting value proposition…', 'Designing feature sections…',
    'Polishing copy…', 'Almost ready…'
  ]);

  try {
    const result = await api('POST', `/api/runs/${state.currentRun.id}/landing`);
    stopCycling(_t3);
    state.currentRun.landing = result.landing;
    state.currentRun.landing_html = result.html;
    state.currentStep = 3;
    renderWizard(state.currentRun);
  } catch (err) {
    stopCycling(_t3);
    showToast(err.message || 'Failed to generate landing page');
    btn.disabled = false;
    btn.innerHTML = 'Generate Landing Page <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6"/></svg>';
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 3: Landing Page Preview
// ─────────────────────────────────────────────────────────────────────────────
function renderStep3(run) {
  const html = run.landing_html || (run.landing && run.landing.html) || '';

  return `
    <div class="fade-in">
      <div class="bg-white rounded-2xl shadow-md p-6 mb-4">
        <div class="flex items-center justify-between gap-4 flex-wrap">
          <div>
            <h2 class="text-xl font-bold text-gray-900 mb-1">Landing Page Preview</h2>
            <p class="text-gray-500 text-sm">This is your AI-generated landing page. Review it before validation.</p>
          </div>
          <button onclick="regenerateLanding()"
            id="regen-landing-btn"
            class="inline-flex items-center gap-2 px-4 py-2 border border-gray-300 text-gray-700 hover:bg-gray-50 font-semibold rounded-xl transition text-sm">
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>
            </svg>
            Regenerate
          </button>
        </div>
      </div>

      <div class="bg-white rounded-2xl shadow-md overflow-hidden mb-4">
        ${html
          ? `<iframe class="landing-iframe" srcdoc="${escapeAttr(html)}" sandbox="allow-same-origin allow-scripts"></iframe>`
          : `<div class="landing-iframe flex items-center justify-center text-gray-400">No landing page HTML available.</div>`
        }
      </div>

      <div class="bg-white rounded-2xl shadow-md p-6 flex justify-end">
        <button onclick="goToValidate()"
          class="inline-flex items-center gap-2 px-6 py-3 bg-purple-600 hover:bg-purple-700 text-white font-semibold rounded-xl transition">
          Run 100-Persona Validation
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6"/>
          </svg>
        </button>
      </div>
    </div>`;
}

async function regenerateLanding() {
  const btn    = document.getElementById('regen-landing-btn');
  btn.disabled = true;
  const _t4 = startCycling('regen-landing-btn', [
    'Writing headline…', 'Crafting value proposition…', 'Designing feature sections…',
    'Polishing copy…', 'Almost ready…'
  ]);

  try {
    const result = await api('POST', `/api/runs/${state.currentRun.id}/landing`);
    stopCycling(_t4);
    state.currentRun.landing      = result.landing;
    state.currentRun.landing_html = result.html;
    renderWizard(state.currentRun);
  } catch (err) {
    stopCycling(_t4);
    showToast(err.message || 'Failed to regenerate landing page');
    btn.disabled = false;
    btn.innerHTML = `
      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
          d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>
      </svg>
      Regenerate`;
  }
}

function goToValidate() {
  state.currentStep = 4;
  state.personas    = [];
  state.validations = {};
  renderWizard(state.currentRun);
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 4: Simulation
// ─────────────────────────────────────────────────────────────────────────────
function renderStep4(run) {
  return `
    <div class="fade-in space-y-4">
      <!-- Progress -->
      <div class="bg-white rounded-2xl shadow-md p-6">
        <div class="flex items-center justify-between mb-3">
          <h2 class="text-xl font-bold text-gray-900">Persona Simulation</h2>
          <span id="progress-label" class="text-sm text-gray-500">Starting…</span>
        </div>
        <div class="progress-bar">
          <div class="progress-fill" id="progress-fill" style="width:0%"></div>
        </div>
        <p id="progress-message" class="text-sm text-gray-500 mt-2">Initializing simulation…</p>
      </div>

      <!-- Live Stats -->
      <div class="bg-white rounded-xl shadow-sm px-6 py-4" id="live-stats">
        <div class="flex flex-wrap gap-6 text-sm font-medium text-gray-700">
          <span>Converting: <span id="stat-converting" class="text-green-600 font-bold">0</span></span>
          <span>Rejecting: <span id="stat-rejecting" class="text-red-600 font-bold">0</span></span>
          <span>Pending: <span id="stat-pending" class="text-gray-500 font-bold">0</span></span>
          <span>Rate: <span id="stat-rate" class="text-purple-700 font-bold">—</span></span>
        </div>
      </div>

      <!-- Persona Grid -->
      <div>
        <h3 class="text-sm font-semibold text-gray-600 uppercase tracking-wide mb-3">Personas</h3>
        <div class="persona-grid" id="persona-grid">
          <div id="persona-empty" class="col-span-full text-center py-8 text-gray-400 text-sm">
            Personas will appear as simulation runs…
          </div>
        </div>
      </div>

      <!-- Results Panel (hidden until results event) -->
      <div id="results-panel" class="hidden"></div>
      <!-- Summary Panel (hidden until summary event) -->
      <div id="summary-panel" class="hidden"></div>
    </div>`;
}

function renderCachedSimulation(run) {
  // Render all persona cards and results from cached data
  (state.personas || []).forEach(p => addPersonaCard(p));
  Object.entries(state.validations || {}).forEach(([pid, v]) => {
    updatePersonaCard(parseInt(pid), v);
  });
  if (run.results) {
    showResultsPanel(run.results);
    updateProgressBar(100, 'Simulation complete');
    if (run.results.summary) showSummaryPanel(run.results.summary);
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// SSE Simulation
// ─────────────────────────────────────────────────────────────────────────────
async function startSimulation(runId) {
  state.personas    = [];
  state.validations = {};

  if (state.eventSource) {
    state.eventSource.close();
  }

  state.eventSource = new EventSource(`/api/runs/${runId}/simulate`);

  state.eventSource.onmessage = (e) => {
    let parsed;
    try {
      parsed = JSON.parse(e.data);
    } catch (_) {
      return;
    }
    const { type, payload } = parsed;
    switch (type) {
      case 'progress':
        updateProgressBar(payload.percent, payload.message);
        break;
      case 'persona':
        state.personas.push(payload);
        addPersonaCard(payload);
        break;
      case 'persona_validation':
        state.validations[payload.personaId] = payload.validation;
        updatePersonaCard(payload.personaId, payload.validation);
        updateLiveStats();
        break;
      case 'results':
        showResultsPanel(payload);
        updateProgressBar(100, 'Simulation complete!');
        state.eventSource.close();
        state.eventSource = null;
        if (state.currentRun) {
          state.currentRun.results = payload;
          state.currentRun.status  = 'complete';
        }
        break;
      case 'summary':
        showSummaryPanel(payload.text);
        break;
      case 'error':
        showToast(payload.message || 'Simulation error');
        state.eventSource.close();
        state.eventSource = null;
        break;
    }
  };

  state.eventSource.onerror = () => {
    // Stream ended or connection error — close cleanly
    if (state.eventSource) {
      state.eventSource.close();
      state.eventSource = null;
    }
  };
}

function updateProgressBar(percent, message) {
  const fill  = document.getElementById('progress-fill');
  const label = document.getElementById('progress-label');
  const msg   = document.getElementById('progress-message');
  if (fill)  fill.style.width  = `${Math.min(100, percent)}%`;
  if (label) label.textContent = `${Math.round(percent)}%`;
  if (msg)   msg.textContent   = message || '';
}

function addPersonaCard(persona) {
  const grid  = document.getElementById('persona-grid');
  if (!grid) return;

  // Remove empty state
  const empty = document.getElementById('persona-empty');
  if (empty) empty.remove();

  const card = document.createElement('div');
  card.id = `persona-card-${persona.id}`;
  card.className = 'persona-card bg-white rounded-xl shadow-sm p-4 fade-in cursor-pointer hover:shadow-md transition';
  card.innerHTML = personaCardInnerHTML(persona, null);
  card.addEventListener('click', () => openPersonaModal(persona.id));
  grid.appendChild(card);
}

function updatePersonaCard(personaId, validation) {
  const card = document.getElementById(`persona-card-${personaId}`);
  if (!card) return;

  const persona = state.personas.find(p => p.id === personaId);
  card.classList.add(validation.converted ? 'converted' : 'rejected');
  card.innerHTML = personaCardInnerHTML(persona || { id: personaId, name: `Persona ${personaId}` }, validation);
  card.onclick = () => openPersonaModal(personaId);
}

function personaCardInnerHTML(persona, validation) {
  const avatars = ['🧑','👩','👨','🧔','👩‍💼','👨‍💼','🧑‍💻','👩‍💻','👨‍💻','🧑‍🎨'];
  const avatar  = avatars[(persona.id || 0) % avatars.length];

  const archetypeColors = {
    power_user: 'bg-purple-100 text-purple-700',
    struggling:  'bg-orange-100 text-orange-700',
    casual:      'bg-blue-100 text-blue-700',
    non_user:    'bg-gray-100 text-gray-500',
  };
  const archCls = archetypeColors[persona.archetype] || 'bg-gray-100 text-gray-500';

  const painLevel  = persona.pain_level || 0;
  const painPct    = (painLevel / 10) * 100;
  const painColor  = painLevel >= 7 ? '#ef4444' : painLevel >= 4 ? '#f97316' : '#22c55e';

  let validationHtml = '';
  if (validation) {
    const isConvert = validation.converted;
    const icon      = isConvert ? '✅' : '❌';
    const label     = isConvert ? 'CONVERTED' : 'REJECTED';
    const labelCls  = isConvert ? 'text-green-700 font-bold' : 'text-red-700 font-bold';
    const reasoning = (validation.reasoning || '').slice(0, 100) + (validation.reasoning && validation.reasoning.length > 100 ? '…' : '');
    const timeline  = validation.decision_timeline || '';

    validationHtml = `
      <div class="mt-3 pt-3 border-t border-gray-100">
        <p class="${labelCls} text-xs">${icon} ${label}</p>
        ${reasoning ? `<p class="text-gray-500 text-xs mt-1 line-clamp-2">"${escapeHtml(reasoning)}"</p>` : ''}
        ${timeline  ? `<p class="text-gray-400 text-xs mt-1">Timeline: ${escapeHtml(timeline)}</p>` : ''}
      </div>`;
  }

  return `
    <div class="flex items-start gap-2 mb-3">
      <span class="text-2xl">${avatar}</span>
      <div class="min-w-0">
        <p class="font-semibold text-gray-800 text-sm truncate">${escapeHtml(persona.name || 'Unknown')}</p>
        <p class="text-gray-500 text-xs">${escapeHtml(persona.role || '')}${persona.age ? `, ${persona.age}` : ''}</p>
      </div>
    </div>
    <div class="mb-2">
      <div class="flex items-center justify-between text-xs text-gray-500 mb-1">
        <span>Pain Level</span>
        <span class="font-medium">${painLevel}/10</span>
      </div>
      <div class="metric-bar">
        <div class="metric-fill" style="width:${painPct}%; background:${painColor}"></div>
      </div>
    </div>
    ${persona.archetype ? `<span class="inline-block px-2 py-0.5 rounded-full text-xs font-medium ${archCls}">${persona.archetype.replace('_', ' ')}</span>` : ''}
    ${validationHtml}`;
}

function updateLiveStats() {
  const total      = state.personas.length;
  const validated  = Object.keys(state.validations).length;
  const converting = Object.values(state.validations).filter(v => v.converted).length;
  const rejecting  = validated - converting;
  const pending    = total - validated;
  const rate       = validated > 0 ? Math.round((converting / validated) * 100) : null;

  const el = (id) => document.getElementById(id);
  if (el('stat-converting')) el('stat-converting').textContent = converting;
  if (el('stat-rejecting'))  el('stat-rejecting').textContent  = rejecting;
  if (el('stat-pending'))    el('stat-pending').textContent    = pending;
  if (el('stat-rate'))       el('stat-rate').textContent       = rate !== null ? `${rate}%` : '—';
}

function showResultsPanel(results) {
  const panel = document.getElementById('results-panel');
  if (!panel) return;
  panel.classList.remove('hidden');

  const rate     = results.conversion_rate || 0;
  const ratePct  = Math.round(rate * 100);
  const rateColor = ratePct > 15 ? 'text-green-600' : ratePct >= 5 ? 'text-yellow-600' : 'text-red-600';
  const rateBg    = ratePct > 15 ? 'bg-green-50 border-green-200' : ratePct >= 5 ? 'bg-yellow-50 border-yellow-200' : 'bg-red-50 border-red-200';

  const ciLow   = results.ci_low   ? Math.round(results.ci_low  * 100) : null;
  const ciHigh  = results.ci_high  ? Math.round(results.ci_high * 100) : null;
  const ciStr   = (ciLow !== null && ciHigh !== null) ? `${ciLow}% – ${ciHigh}%` : 'N/A';

  const intentPct = results.intent_strength ? Math.round((results.intent_strength / 10) * 100) : 0;
  const composite = results.composite_score ? results.composite_score.toFixed(2) : 'N/A';

  // Collect top friction points
  const frictionMap = {};
  Object.values(state.validations).forEach(v => {
    (v.friction_points || []).forEach(fp => {
      const key = (fp || '').trim().toLowerCase();
      if (key) frictionMap[key] = (frictionMap[key] || 0) + 1;
    });
  });
  const topFriction = Object.entries(frictionMap)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 5);

  panel.innerHTML = `
    <div class="bg-white rounded-2xl shadow-md p-8 fade-in">
      <h2 class="text-xl font-bold text-gray-900 mb-6">Validation Results</h2>

      <!-- Conversion rate hero -->
      <div class="border rounded-2xl p-6 mb-6 text-center ${rateBg}">
        <p class="text-sm font-medium text-gray-600 mb-1">Conversion Rate</p>
        <p class="text-6xl font-black ${rateColor}">${ratePct}%</p>
        <p class="text-sm text-gray-500 mt-2">
          ${results.conversions || 0} converted / ${results.total || 0} total &nbsp;·&nbsp;
          CI: ${ciStr}
        </p>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-6">
        <!-- Intent strength -->
        <div class="bg-gray-50 rounded-xl p-4">
          <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">Avg Intent Strength</p>
          <p class="text-2xl font-bold text-gray-800 mb-2">${results.intent_strength ? results.intent_strength.toFixed(1) : '—'}<span class="text-sm font-normal text-gray-400">/10</span></p>
          <div class="metric-bar">
            <div class="metric-fill" style="width:${intentPct}%"></div>
          </div>
        </div>

        <!-- Composite score -->
        <div class="bg-gray-50 rounded-xl p-4">
          <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">Composite Score</p>
          <p class="text-2xl font-bold text-purple-700">${composite}</p>
          <p class="text-xs text-gray-400 mt-1">Combined quality metric</p>
        </div>

        <!-- Avg scores -->
        <div class="bg-gray-50 rounded-xl p-4">
          <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">Avg Impression / Relevance</p>
          <p class="text-2xl font-bold text-gray-800">
            ${results.avg_impression ? results.avg_impression.toFixed(1) : '—'}
            <span class="text-sm font-normal text-gray-400">/ ${results.avg_relevance ? results.avg_relevance.toFixed(1) : '—'}</span>
          </p>
          <p class="text-xs text-gray-400 mt-1">Out of 10</p>
        </div>
      </div>

      <!-- Friction points -->
      ${topFriction.length > 0 ? `
        <div class="border border-gray-200 rounded-xl p-5">
          <h3 class="text-sm font-bold text-gray-700 uppercase tracking-wide mb-3">Top Friction Points</h3>
          <div class="space-y-2">
            ${topFriction.map(([fp, count]) => `
              <div class="flex items-center gap-3">
                <div class="flex-1 min-w-0">
                  <p class="text-sm text-gray-700 truncate">${escapeHtml(fp)}</p>
                </div>
                <span class="shrink-0 text-xs font-semibold bg-red-50 text-red-700 px-2 py-0.5 rounded-full">${count}x</span>
              </div>`).join('')}
          </div>
        </div>
      ` : ''}

      <div class="mt-6 flex justify-center">
        <a href="#/" class="inline-flex items-center gap-2 px-5 py-2.5 border border-gray-300 text-gray-700 hover:bg-gray-50 font-semibold rounded-xl transition text-sm">
          ← Back to All Ideas
        </a>
      </div>
    </div>`;
}

// ─────────────────────────────────────────────────────────────────────────────
// Utility: HTML escaping
// ─────────────────────────────────────────────────────────────────────────────
function escapeHtml(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// For use in HTML attribute values that are already inside double-quotes
function escapeAttr(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;');
}

// ─────────────────────────────────────────────────────────────────────────────
// Persona Detail Modal
// ─────────────────────────────────────────────────────────────────────────────
function openPersonaModal(personaId) {
  const persona    = state.personas.find(p => p.id === personaId);
  const validation = state.validations[personaId];
  if (!persona) return;

  const avatars = ['🧑','👩','👨','🧔','👩‍💼','👨‍💼','🧑‍💻','👩‍💻','👨‍💻','🧑‍🎨'];
  const avatar  = avatars[(persona.id || 0) % avatars.length];

  const dl = persona.daily_life || {};
  const struggles = (dl.top_struggles || []).map(s => `<li class="text-gray-600 text-sm">${escapeHtml(s)}</li>`).join('');
  const priorities = (dl.current_priorities || []).map(s => `<li class="text-gray-600 text-sm">${escapeHtml(s)}</li>`).join('');

  let validationSection = '';
  if (validation) {
    const isConvert = validation.converted;
    const fp = (validation.friction_points || []).map(f => `<li class="text-gray-600 text-sm">${escapeHtml(f)}</li>`).join('');
    validationSection = `
      <div class="mt-4 pt-4 border-t border-gray-100">
        <div class="flex items-center gap-2 mb-3">
          <span class="text-lg">${isConvert ? '✅' : '❌'}</span>
          <span class="font-bold ${isConvert ? 'text-green-700' : 'text-red-700'}">${isConvert ? 'CONVERTED' : 'REJECTED'}</span>
          ${validation.decision_timeline ? `<span class="text-gray-400 text-xs ml-auto">${escapeHtml(validation.decision_timeline)}</span>` : ''}
        </div>
        ${validation.reasoning ? `
          <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Reasoning</p>
          <p class="text-gray-700 text-sm mb-3">${escapeHtml(validation.reasoning)}</p>` : ''}
        ${fp ? `
          <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Friction Points</p>
          <ul class="list-disc list-inside space-y-1 mb-3">${fp}</ul>` : ''}
        <div class="grid grid-cols-3 gap-2 text-center text-xs">
          ${validation.intent_strength != null ? `<div class="bg-gray-50 rounded-lg p-2"><div class="font-bold text-gray-800">${validation.intent_strength}/10</div><div class="text-gray-500">Intent</div></div>` : ''}
          ${validation.impression_score != null ? `<div class="bg-gray-50 rounded-lg p-2"><div class="font-bold text-gray-800">${validation.impression_score}/10</div><div class="text-gray-500">Impression</div></div>` : ''}
          ${validation.relevance_score  != null ? `<div class="bg-gray-50 rounded-lg p-2"><div class="font-bold text-gray-800">${validation.relevance_score}/10</div><div class="text-gray-500">Relevance</div></div>` : ''}
        </div>
        ${validation.pricing_reaction ? `<p class="text-xs text-gray-500 mt-2">💰 ${escapeHtml(validation.pricing_reaction)}</p>` : ''}
      </div>`;
  }

  const backdrop = document.createElement('div');
  backdrop.className = 'modal-backdrop fade-in';
  backdrop.id = 'persona-modal';
  backdrop.innerHTML = `
    <div class="modal-box max-w-lg w-full mx-4 overflow-y-auto max-h-[90vh]" onclick="event.stopPropagation()">
      <div class="flex items-center justify-between mb-4">
        <div class="flex items-center gap-3">
          <span class="text-4xl">${avatar}</span>
          <div>
            <h2 class="text-xl font-bold text-gray-900">${escapeHtml(persona.name || 'Persona')}</h2>
            <p class="text-gray-500 text-sm">${escapeHtml(persona.role || '')}${persona.age ? `, ${persona.age}` : ''}${persona.company_size ? ` · ${escapeHtml(persona.company_size)}` : ''}</p>
          </div>
        </div>
        <button onclick="closePersonaModal()" class="text-gray-400 hover:text-gray-600 text-2xl leading-none">&times;</button>
      </div>

      <div class="grid grid-cols-2 gap-3 mb-4 text-sm">
        ${persona.pain_level != null ? `<div class="bg-gray-50 rounded-lg p-3"><div class="text-xs text-gray-500 mb-1">Pain Level</div><div class="font-semibold text-gray-800">${persona.pain_level}/10</div></div>` : ''}
        ${persona.skepticism  != null ? `<div class="bg-gray-50 rounded-lg p-3"><div class="text-xs text-gray-500 mb-1">Skepticism</div><div class="font-semibold text-gray-800">${persona.skepticism}/10</div></div>` : ''}
        ${persona.budget ? `<div class="bg-gray-50 rounded-lg p-3"><div class="text-xs text-gray-500 mb-1">Budget</div><div class="font-semibold text-gray-800">${escapeHtml(persona.budget)}</div></div>` : ''}
        ${persona.decision_authority ? `<div class="bg-gray-50 rounded-lg p-3"><div class="text-xs text-gray-500 mb-1">Decision Authority</div><div class="font-semibold text-gray-800">${escapeHtml(persona.decision_authority)}</div></div>` : ''}
      </div>

      ${persona.current_workflow ? `
        <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Current Workflow</p>
        <p class="text-gray-700 text-sm mb-3">${escapeHtml(persona.current_workflow)}</p>` : ''}

      ${persona.current_tools ? `
        <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Tools They Use</p>
        <p class="text-gray-700 text-sm mb-3">${escapeHtml(persona.current_tools)}</p>` : ''}

      ${dl.daily_routine ? `
        <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Typical Day</p>
        <p class="text-gray-700 text-sm mb-3 whitespace-pre-line">${escapeHtml(dl.daily_routine)}</p>` : ''}

      ${struggles ? `
        <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Top Struggles</p>
        <ul class="list-disc list-inside space-y-1 mb-3">${struggles}</ul>` : ''}

      ${dl.mental_state ? `
        <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Mental State</p>
        <p class="text-gray-700 text-sm mb-3">${escapeHtml(dl.mental_state)}</p>` : ''}

      ${dl.discovery_context ? `
        <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">How They'd Discover This</p>
        <p class="text-gray-700 text-sm mb-3">${escapeHtml(dl.discovery_context)}</p>` : ''}

      ${priorities ? `
        <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Current Priorities</p>
        <ul class="list-disc list-inside space-y-1 mb-3">${priorities}</ul>` : ''}

      ${persona.personality ? `
        <p class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1">Personality</p>
        <p class="text-gray-700 text-sm mb-3">${escapeHtml(persona.personality)}</p>` : ''}

      ${validationSection}
    </div>`;

  backdrop.addEventListener('click', e => { if (e.target === backdrop) closePersonaModal(); });
  document.body.appendChild(backdrop);
}

function closePersonaModal() {
  const m = document.getElementById('persona-modal');
  if (m) m.remove();
}

// ─────────────────────────────────────────────────────────────────────────────
// Summary Panel
// ─────────────────────────────────────────────────────────────────────────────
function showSummaryPanel(text) {
  const panel = document.getElementById('summary-panel');
  if (!panel || !text) return;
  panel.classList.remove('hidden');
  const paragraphs = text.split(/\n+/).filter(p => p.trim()).map(p =>
    `<p class="text-gray-700 text-sm leading-relaxed">${escapeHtml(p.trim())}</p>`
  ).join('');
  panel.innerHTML = `
    <div class="bg-white rounded-2xl shadow-md p-6 mt-6 fade-in">
      <div class="flex items-center gap-2 mb-4">
        <span class="text-2xl">🧠</span>
        <h3 class="text-lg font-bold text-gray-900">Strategic Analysis</h3>
      </div>
      <div class="space-y-3">${paragraphs}</div>
    </div>`;
}
