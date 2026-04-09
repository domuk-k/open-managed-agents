// OMA Dashboard — Vanilla JS SPA
(function () {
  'use strict';

  // ---------------------------------------------------------------------------
  // API client
  // ---------------------------------------------------------------------------
  const API = '/v1';

  async function api(method, path, body) {
    const opts = { method, headers: { 'Content-Type': 'application/json' } };
    if (body) opts.body = JSON.stringify(body);
    const res = await fetch(API + path, opts);
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: { message: res.statusText } }));
      throw new Error((err.error && err.error.message) || res.statusText);
    }
    return res.json();
  }

  // ---------------------------------------------------------------------------
  // Toast notifications
  // ---------------------------------------------------------------------------
  function toast(msg, type) {
    type = type || 'success';
    const el = document.createElement('div');
    el.className = 'toast toast-' + type;
    el.textContent = msg;
    document.getElementById('toasts').appendChild(el);
    setTimeout(function () { el.remove(); }, 3000);
  }

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------
  function $(sel, ctx) { return (ctx || document).querySelector(sel); }
  function $$(sel, ctx) { return Array.from((ctx || document).querySelectorAll(sel)); }

  function shortId(id) { return id ? id.substring(0, 8) : '-'; }

  function fmtDate(d) {
    if (!d) return '-';
    var dt = new Date(d);
    return dt.toLocaleDateString() + ' ' + dt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function badge(status) {
    return '<span class="badge badge-' + status + '">' + status + '</span>';
  }

  function escHtml(str) {
    if (!str) return '';
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  // ---------------------------------------------------------------------------
  // Router
  // ---------------------------------------------------------------------------
  var currentView = null;
  var refreshTimer = null;
  var currentSSE = null;

  function navigate() {
    if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null; }
    if (currentSSE) { currentSSE.close(); currentSSE = null; }

    var hash = location.hash.replace('#', '') || 'dashboard';
    var parts = hash.split('/');
    var route = parts[0];
    var id = parts[1] || null;

    // Update nav active
    $$('nav a').forEach(function (a) {
      a.classList.toggle('active', a.getAttribute('data-route') === route);
    });

    currentView = route;
    var app = document.getElementById('app');

    switch (route) {
      case 'dashboard': renderDashboard(app); break;
      case 'agents':
        if (id) renderAgentDetail(app, id);
        else renderAgents(app);
        break;
      case 'environments':
        if (id) renderEnvironmentDetail(app, id);
        else renderEnvironments(app);
        break;
      case 'sessions':
        if (id) renderSessionDetail(app, id);
        else renderSessions(app);
        break;
      default: renderDashboard(app);
    }
  }

  window.addEventListener('hashchange', navigate);
  window.addEventListener('load', navigate);

  // ---------------------------------------------------------------------------
  // Dashboard view
  // ---------------------------------------------------------------------------
  function renderDashboard(app) {
    app.innerHTML = '<div class="spinner"></div>';
    Promise.all([api('GET', '/agents'), api('GET', '/environments'), api('GET', '/sessions')])
      .then(function (res) {
        var agents = res[0] || [];
        var envs = res[1] || [];
        var sessions = res[2] || [];
        var active = sessions.filter(function (s) { return s.status === 'running' || s.status === 'starting'; });

        app.innerHTML =
          '<h1 style="margin-bottom:20px">Dashboard</h1>' +
          '<div class="cards">' +
            '<div class="card"><div class="card-label">Agents</div><div class="card-value accent">' + agents.length + '</div></div>' +
            '<div class="card"><div class="card-label">Environments</div><div class="card-value accent">' + envs.length + '</div></div>' +
            '<div class="card"><div class="card-label">Total Sessions</div><div class="card-value">' + sessions.length + '</div></div>' +
            '<div class="card"><div class="card-label">Active Sessions</div><div class="card-value green">' + active.length + '</div></div>' +
          '</div>' +
          '<div class="section">' +
            '<div class="section-header"><h2>Recent Sessions</h2></div>' +
            '<div class="section-body">' + renderSessionTable(sessions.slice(-10).reverse()) + '</div>' +
          '</div>';
      })
      .catch(function (err) { app.innerHTML = '<p>Error loading dashboard: ' + escHtml(err.message) + '</p>'; });
  }

  // ---------------------------------------------------------------------------
  // Agents view
  // ---------------------------------------------------------------------------
  function renderAgents(app) {
    app.innerHTML = '<div class="spinner"></div>';
    api('GET', '/agents').then(function (agents) {
      agents = agents || [];
      app.innerHTML =
        '<div class="section">' +
          '<div class="section-header"><h2>Agents</h2><button class="btn btn-primary" id="btnNewAgent">+ New Agent</button></div>' +
          '<div class="section-body">' +
            (agents.length === 0
              ? '<div class="empty-state"><p>No agents yet. Create one to get started.</p></div>'
              : '<table><thead><tr><th>ID</th><th>Name</th><th>Model</th><th>Version</th><th>Created</th><th></th></tr></thead><tbody>' +
                agents.map(function (a) {
                  return '<tr>' +
                    '<td class="id-cell"><a href="#agents/' + a.id + '">' + shortId(a.id) + '</a></td>' +
                    '<td><a href="#agents/' + a.id + '">' + escHtml(a.name) + '</a></td>' +
                    '<td>' + escHtml(a.model && a.model.id || '-') + '</td>' +
                    '<td>v' + a.version + '</td>' +
                    '<td>' + fmtDate(a.created_at) + '</td>' +
                    '<td>' + (a.archived_at ? '<span class="badge badge-completed">archived</span>' : '<button class="btn btn-danger btn-sm btn-archive-agent" data-id="' + a.id + '">Archive</button>') + '</td>' +
                    '</tr>';
                }).join('') +
                '</tbody></table>'
            ) +
          '</div>' +
        '</div>' +
        '<div class="section" id="agentForm" style="display:none">' +
          '<div class="section-header"><h2>Create Agent</h2></div>' +
          '<div class="section-body">' +
            '<form id="createAgentForm">' +
              '<div class="form-row">' +
                '<div class="form-group"><label>Name</label><input name="name" required placeholder="e.g. Coder"></div>' +
                '<div class="form-group"><label>Model ID</label><input name="model_id" required placeholder="e.g. openai/gpt-4o"></div>' +
              '</div>' +
              '<div class="form-group"><label>System Prompt</label><textarea name="system" placeholder="You are a helpful assistant..."></textarea></div>' +
              '<div class="form-group"><label>Description</label><input name="description" placeholder="Optional description"></div>' +
              '<button class="btn btn-primary" type="submit">Create Agent</button> ' +
              '<button class="btn" type="button" id="cancelAgent">Cancel</button>' +
            '</form>' +
          '</div>' +
        '</div>';

      $('#btnNewAgent').addEventListener('click', function () { $('#agentForm').style.display = ''; });
      $('#cancelAgent').addEventListener('click', function () { $('#agentForm').style.display = 'none'; });

      $('#createAgentForm').addEventListener('submit', function (e) {
        e.preventDefault();
        var f = e.target;
        var body = {
          name: f.name.value,
          model: { id: f.model_id.value },
          tools: []
        };
        if (f.system.value) body.system = f.system.value;
        if (f.description.value) body.description = f.description.value;
        api('POST', '/agents', body)
          .then(function () { toast('Agent created'); navigate(); })
          .catch(function (err) { toast(err.message, 'error'); });
      });

      $$('.btn-archive-agent').forEach(function (btn) {
        btn.addEventListener('click', function () {
          if (!confirm('Archive this agent?')) return;
          api('POST', '/agents/' + btn.dataset.id + '/archive')
            .then(function () { toast('Agent archived'); navigate(); })
            .catch(function (err) { toast(err.message, 'error'); });
        });
      });
    }).catch(function (err) { app.innerHTML = '<p>Error: ' + escHtml(err.message) + '</p>'; });
  }

  function renderAgentDetail(app, id) {
    app.innerHTML = '<div class="spinner"></div>';
    api('GET', '/agents/' + id).then(function (a) {
      app.innerHTML =
        '<a href="#agents" class="back-link">&larr; All Agents</a>' +
        '<div class="detail-header"><div>' +
          '<h1>' + escHtml(a.name) + '</h1>' +
          '<div class="detail-meta"><span>v' + a.version + '</span><span>' + escHtml(a.model && a.model.id || '-') + '</span></div>' +
        '</div>' +
        (a.archived_at ? '<span class="badge badge-completed">archived</span>' : '<button class="btn btn-danger btn-sm" id="btnArchiveAgent">Archive</button>') +
        '</div>' +
        '<div class="section"><div class="section-header"><h2>Details</h2></div><div class="section-body">' +
          '<dl class="kv-grid">' +
            '<dt>ID</dt><dd>' + a.id + '</dd>' +
            '<dt>Name</dt><dd>' + escHtml(a.name) + '</dd>' +
            '<dt>Model</dt><dd>' + escHtml(a.model && a.model.id || '-') + '</dd>' +
            '<dt>System Prompt</dt><dd>' + escHtml(a.system || '(none)') + '</dd>' +
            '<dt>Description</dt><dd>' + escHtml(a.description || '(none)') + '</dd>' +
            '<dt>Tools</dt><dd>' + (a.tools && a.tools.length ? a.tools.map(function (t) { return t.type; }).join(', ') : '(none)') + '</dd>' +
            '<dt>Created</dt><dd>' + fmtDate(a.created_at) + '</dd>' +
            '<dt>Updated</dt><dd>' + fmtDate(a.updated_at) + '</dd>' +
          '</dl>' +
        '</div></div>' +
        '<div class="section"><div class="section-header"><h2>Metadata</h2></div><div class="section-body">' +
          '<pre style="font-family:var(--font-mono);font-size:13px;color:var(--text-secondary)">' + escHtml(JSON.stringify(a.metadata || {}, null, 2)) + '</pre>' +
        '</div></div>';

      var archiveBtn = $('#btnArchiveAgent');
      if (archiveBtn) {
        archiveBtn.addEventListener('click', function () {
          if (!confirm('Archive this agent?')) return;
          api('POST', '/agents/' + id + '/archive')
            .then(function () { toast('Agent archived'); navigate(); })
            .catch(function (err) { toast(err.message, 'error'); });
        });
      }
    }).catch(function (err) { app.innerHTML = '<p>Error: ' + escHtml(err.message) + '</p>'; });
  }

  // ---------------------------------------------------------------------------
  // Environments view
  // ---------------------------------------------------------------------------
  function renderEnvironments(app) {
    app.innerHTML = '<div class="spinner"></div>';
    api('GET', '/environments').then(function (envs) {
      envs = envs || [];
      app.innerHTML =
        '<div class="section">' +
          '<div class="section-header"><h2>Environments</h2><button class="btn btn-primary" id="btnNewEnv">+ New Environment</button></div>' +
          '<div class="section-body">' +
            (envs.length === 0
              ? '<div class="empty-state"><p>No environments yet.</p></div>'
              : '<table><thead><tr><th>ID</th><th>Name</th><th>Type</th><th>Networking</th><th>Created</th><th></th></tr></thead><tbody>' +
                envs.map(function (e) {
                  return '<tr>' +
                    '<td class="id-cell"><a href="#environments/' + e.id + '">' + shortId(e.id) + '</a></td>' +
                    '<td><a href="#environments/' + e.id + '">' + escHtml(e.name) + '</a></td>' +
                    '<td>' + escHtml(e.config && e.config.type || '-') + '</td>' +
                    '<td>' + escHtml(e.config && e.config.networking && e.config.networking.type || '-') + '</td>' +
                    '<td>' + fmtDate(e.created_at) + '</td>' +
                    '<td>' + (e.archived_at ? '<span class="badge badge-completed">archived</span>' : '<button class="btn btn-danger btn-sm btn-archive-env" data-id="' + e.id + '">Archive</button>') + '</td>' +
                    '</tr>';
                }).join('') +
                '</tbody></table>'
            ) +
          '</div>' +
        '</div>' +
        '<div class="section" id="envForm" style="display:none">' +
          '<div class="section-header"><h2>Create Environment</h2></div>' +
          '<div class="section-body">' +
            '<form id="createEnvForm">' +
              '<div class="form-row">' +
                '<div class="form-group"><label>Name</label><input name="name" required placeholder="e.g. sandbox"></div>' +
                '<div class="form-group"><label>Type</label><select name="type"><option value="local">local</option><option value="docker">docker</option></select></div>' +
              '</div>' +
              '<div class="form-group"><label>Networking</label><select name="networking"><option value="none">none</option><option value="full">full</option><option value="restricted">restricted</option></select></div>' +
              '<button class="btn btn-primary" type="submit">Create Environment</button> ' +
              '<button class="btn" type="button" id="cancelEnv">Cancel</button>' +
            '</form>' +
          '</div>' +
        '</div>';

      $('#btnNewEnv').addEventListener('click', function () { $('#envForm').style.display = ''; });
      $('#cancelEnv').addEventListener('click', function () { $('#envForm').style.display = 'none'; });

      $('#createEnvForm').addEventListener('submit', function (ev) {
        ev.preventDefault();
        var f = ev.target;
        api('POST', '/environments', {
          name: f.name.value,
          config: { type: f.type.value, networking: { type: f.networking.value } }
        })
          .then(function () { toast('Environment created'); navigate(); })
          .catch(function (err) { toast(err.message, 'error'); });
      });

      $$('.btn-archive-env').forEach(function (btn) {
        btn.addEventListener('click', function () {
          if (!confirm('Archive this environment?')) return;
          api('POST', '/environments/' + btn.dataset.id + '/archive')
            .then(function () { toast('Archived'); navigate(); })
            .catch(function (err) { toast(err.message, 'error'); });
        });
      });
    }).catch(function (err) { app.innerHTML = '<p>Error: ' + escHtml(err.message) + '</p>'; });
  }

  function renderEnvironmentDetail(app, id) {
    app.innerHTML = '<div class="spinner"></div>';
    api('GET', '/environments/' + id).then(function (e) {
      app.innerHTML =
        '<a href="#environments" class="back-link">&larr; All Environments</a>' +
        '<div class="detail-header"><div>' +
          '<h1>' + escHtml(e.name) + '</h1>' +
          '<div class="detail-meta"><span>' + escHtml(e.config && e.config.type || '-') + '</span></div>' +
        '</div>' +
        (e.archived_at ? '<span class="badge badge-completed">archived</span>' : '<button class="btn btn-danger btn-sm" id="btnArchiveEnv">Archive</button>') +
        '</div>' +
        '<div class="section"><div class="section-header"><h2>Configuration</h2></div><div class="section-body">' +
          '<dl class="kv-grid">' +
            '<dt>ID</dt><dd>' + e.id + '</dd>' +
            '<dt>Name</dt><dd>' + escHtml(e.name) + '</dd>' +
            '<dt>Type</dt><dd>' + escHtml(e.config && e.config.type || '-') + '</dd>' +
            '<dt>Networking</dt><dd>' + escHtml(e.config && e.config.networking && e.config.networking.type || '-') + '</dd>' +
            '<dt>Packages</dt><dd>' + (e.config && e.config.packages && e.config.packages.length ? e.config.packages.join(', ') : '(none)') + '</dd>' +
            '<dt>Created</dt><dd>' + fmtDate(e.created_at) + '</dd>' +
            '<dt>Updated</dt><dd>' + fmtDate(e.updated_at) + '</dd>' +
          '</dl>' +
        '</div></div>' +
        '<div class="section"><div class="section-header"><h2>Full Config</h2></div><div class="section-body">' +
          '<pre style="font-family:var(--font-mono);font-size:13px;color:var(--text-secondary)">' + escHtml(JSON.stringify(e.config || {}, null, 2)) + '</pre>' +
        '</div></div>';

      var archiveBtn = $('#btnArchiveEnv');
      if (archiveBtn) {
        archiveBtn.addEventListener('click', function () {
          if (!confirm('Archive this environment?')) return;
          api('POST', '/environments/' + id + '/archive')
            .then(function () { toast('Archived'); navigate(); })
            .catch(function (err) { toast(err.message, 'error'); });
        });
      }
    }).catch(function (err) { app.innerHTML = '<p>Error: ' + escHtml(err.message) + '</p>'; });
  }

  // ---------------------------------------------------------------------------
  // Sessions view
  // ---------------------------------------------------------------------------
  function renderSessionTable(sessions) {
    if (!sessions || sessions.length === 0) {
      return '<div class="empty-state"><p>No sessions yet.</p></div>';
    }
    return '<table><thead><tr><th>ID</th><th>Title</th><th>Agent</th><th>Status</th><th>Created</th></tr></thead><tbody>' +
      sessions.map(function (s) {
        return '<tr>' +
          '<td class="id-cell"><a href="#sessions/' + s.id + '">' + shortId(s.id) + '</a></td>' +
          '<td><a href="#sessions/' + s.id + '">' + escHtml(s.title || '(untitled)') + '</a></td>' +
          '<td class="id-cell">' + shortId(s.agent) + '</td>' +
          '<td>' + badge(s.status) + '</td>' +
          '<td>' + fmtDate(s.created_at) + '</td>' +
          '</tr>';
      }).join('') +
      '</tbody></table>';
  }

  function renderSessions(app) {
    app.innerHTML = '<div class="spinner"></div>';
    Promise.all([api('GET', '/sessions'), api('GET', '/agents'), api('GET', '/environments')])
      .then(function (res) {
        var sessions = res[0] || [];
        var agents = res[1] || [];
        var envs = res[2] || [];

        app.innerHTML =
          '<div class="section">' +
            '<div class="section-header"><h2>Sessions</h2><button class="btn btn-primary" id="btnNewSession">+ New Session</button></div>' +
            '<div class="section-body" id="sessionListBody">' + renderSessionTable(sessions) + '</div>' +
          '</div>' +
          '<div class="section" id="sessionForm" style="display:none">' +
            '<div class="section-header"><h2>Create Session</h2></div>' +
            '<div class="section-body">' +
              '<form id="createSessionForm">' +
                '<div class="form-row">' +
                  '<div class="form-group"><label>Agent</label><select name="agent_id" required>' +
                    '<option value="">Select agent...</option>' +
                    agents.filter(function (a) { return !a.archived_at; }).map(function (a) { return '<option value="' + a.id + '">' + escHtml(a.name) + ' (' + shortId(a.id) + ')</option>'; }).join('') +
                  '</select></div>' +
                  '<div class="form-group"><label>Environment</label><select name="environment_id" required>' +
                    '<option value="">Select environment...</option>' +
                    envs.filter(function (e) { return !e.archived_at; }).map(function (e) { return '<option value="' + e.id + '">' + escHtml(e.name) + ' (' + shortId(e.id) + ')</option>'; }).join('') +
                  '</select></div>' +
                '</div>' +
                '<div class="form-group"><label>Title</label><input name="title" placeholder="Optional session title"></div>' +
                '<button class="btn btn-primary" type="submit">Create Session</button> ' +
                '<button class="btn" type="button" id="cancelSession">Cancel</button>' +
              '</form>' +
            '</div>' +
          '</div>';

        $('#btnNewSession').addEventListener('click', function () { $('#sessionForm').style.display = ''; });
        $('#cancelSession').addEventListener('click', function () { $('#sessionForm').style.display = 'none'; });

        $('#createSessionForm').addEventListener('submit', function (ev) {
          ev.preventDefault();
          var f = ev.target;
          var body = {
            agent_id: f.agent_id.value,
            environment_id: f.environment_id.value
          };
          if (f.title.value) body.title = f.title.value;
          api('POST', '/sessions', body)
            .then(function () { toast('Session created'); navigate(); })
            .catch(function (err) { toast(err.message, 'error'); });
        });

        // Auto-refresh session list every 5 seconds
        refreshTimer = setInterval(function () {
          if (currentView !== 'sessions') return;
          api('GET', '/sessions').then(function (s) {
            var el = document.getElementById('sessionListBody');
            if (el) el.innerHTML = renderSessionTable(s || []);
          }).catch(function () {});
        }, 5000);
      })
      .catch(function (err) { app.innerHTML = '<p>Error: ' + escHtml(err.message) + '</p>'; });
  }

  // ---------------------------------------------------------------------------
  // Session Detail — with SSE event streaming
  // ---------------------------------------------------------------------------
  function renderSessionDetail(app, id) {
    app.innerHTML = '<div class="spinner"></div>';
    Promise.all([api('GET', '/sessions/' + id), api('GET', '/sessions/' + id + '/events').catch(function () { return []; })])
      .then(function (res) {
        var s = res[0];
        var events = res[1] || [];

        app.innerHTML =
          '<a href="#sessions" class="back-link">&larr; All Sessions</a>' +
          '<div class="detail-header"><div>' +
            '<h1>' + escHtml(s.title || 'Session ' + shortId(s.id)) + '</h1>' +
            '<div class="detail-meta"><span>' + badge(s.status) + '</span><span>Agent: ' + shortId(s.agent) + '</span></div>' +
          '</div>' +
          '<div style="display:flex;gap:8px">' +
            (s.status === 'running' ? '<button class="btn btn-sm" id="btnPause">Pause</button>' : '') +
            (s.status === 'paused' ? '<button class="btn btn-primary btn-sm" id="btnResume">Resume</button>' : '') +
          '</div>' +
          '</div>' +
          '<div class="section"><div class="section-header"><h2>Details</h2></div><div class="section-body">' +
            '<dl class="kv-grid">' +
              '<dt>Session ID</dt><dd>' + s.id + '</dd>' +
              '<dt>Agent ID</dt><dd>' + s.agent + '</dd>' +
              '<dt>Agent Version</dt><dd>v' + s.agent_version + '</dd>' +
              '<dt>Environment</dt><dd>' + s.environment_id + '</dd>' +
              '<dt>Status</dt><dd>' + badge(s.status) + '</dd>' +
              '<dt>Created</dt><dd>' + fmtDate(s.created_at) + '</dd>' +
              '<dt>Updated</dt><dd>' + fmtDate(s.updated_at) + '</dd>' +
              (s.completed_at ? '<dt>Completed</dt><dd>' + fmtDate(s.completed_at) + '</dd>' : '') +
            '</dl>' +
          '</div></div>' +
          '<div class="section"><div class="section-header"><h2>Events <span id="sseStatus" style="font-size:12px;color:var(--text-muted);font-weight:400"></span></h2></div>' +
            '<div class="event-stream" id="eventStream">' +
              (events.length === 0
                ? '<div class="empty-state"><p>No events yet.</p></div>'
                : events.map(renderEventItem).join('')) +
            '</div>' +
          '</div>' +
          '<div class="section" id="evalSection" style="display:none"><div class="section-header"><h2>Evaluation</h2></div>' +
            '<div class="section-body" id="evalBody"></div>' +
          '</div>';

        // Pause / Resume
        var pauseBtn = document.getElementById('btnPause');
        if (pauseBtn) {
          pauseBtn.addEventListener('click', function () {
            api('POST', '/sessions/' + id + '/pause')
              .then(function () { toast('Session paused'); navigate(); })
              .catch(function (err) { toast(err.message, 'error'); });
          });
        }
        var resumeBtn = document.getElementById('btnResume');
        if (resumeBtn) {
          resumeBtn.addEventListener('click', function () {
            api('POST', '/sessions/' + id + '/resume')
              .then(function () { toast('Session resumed'); navigate(); })
              .catch(function (err) { toast(err.message, 'error'); });
          });
        }

        // Load evaluation
        api('GET', '/sessions/' + id + '/evaluation').then(function (evalData) {
          if (evalData && evalData.status === 'evaluated' && evalData.events && evalData.events.length > 0) {
            var sec = document.getElementById('evalSection');
            var body = document.getElementById('evalBody');
            if (sec && body) {
              sec.style.display = '';
              body.innerHTML = '<pre style="font-family:var(--font-mono);font-size:13px;color:var(--text-secondary)">' +
                escHtml(JSON.stringify(evalData.events, null, 2)) + '</pre>';
            }
          }
        }).catch(function () {});

        // SSE streaming for active sessions
        if (s.status === 'running' || s.status === 'starting') {
          startSSE(id);
        }
      })
      .catch(function (err) { app.innerHTML = '<p>Error: ' + escHtml(err.message) + '</p>'; });
  }

  function renderEventItem(evt) {
    var data = '';
    if (evt.data) {
      try {
        var parsed = typeof evt.data === 'string' ? JSON.parse(evt.data) : evt.data;
        data = JSON.stringify(parsed, null, 2);
      } catch (e) {
        data = String(evt.data);
      }
    }
    return '<div class="event-item">' +
      '<span class="event-type">' + escHtml(evt.type || 'unknown') + '</span>' +
      '<span class="event-content">' + escHtml(data) + '</span>' +
      '<span class="event-time">' + (evt.created_at ? fmtDate(evt.created_at) : '') + '</span>' +
      '</div>';
  }

  function startSSE(sessionId) {
    var statusEl = document.getElementById('sseStatus');
    if (statusEl) statusEl.textContent = '(connecting...)';

    var es = new EventSource(API + '/sessions/' + sessionId + '/stream');
    currentSSE = es;

    es.onopen = function () {
      if (statusEl) statusEl.textContent = '(live)';
    };

    es.onmessage = function (e) {
      var stream = document.getElementById('eventStream');
      if (!stream) return;
      // Clear empty state
      var empty = stream.querySelector('.empty-state');
      if (empty) empty.remove();

      try {
        var evt = JSON.parse(e.data);
        var html = '<div class="event-item">' +
          '<span class="event-type">' + escHtml(evt.type || 'event') + '</span>' +
          '<span class="event-content">' + escHtml(JSON.stringify(evt.content || evt, null, 2)) + '</span>' +
          '<span class="event-time">' + new Date().toLocaleTimeString() + '</span>' +
          '</div>';
        stream.insertAdjacentHTML('beforeend', html);
        stream.scrollTop = stream.scrollHeight;
      } catch (err) {
        // ignore parse errors
      }
    };

    es.onerror = function () {
      if (statusEl) statusEl.textContent = '(disconnected)';
      es.close();
      currentSSE = null;
    };
  }
})();
