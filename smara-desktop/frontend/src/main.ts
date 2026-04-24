import {Ask, GetWorkspaces, GetSessions, SwitchSession, GetSessionHistory, CreateSession, SetWorkspace, GetConfig, UpdateConfig, GetTools} from '../wailsjs/go/main/App';
import {EventsOn} from '../wailsjs/runtime';
import {marked} from 'marked';

// Elements
const themeToggle = document.getElementById('theme-toggle');
const body = document.body;
const chatInput = document.getElementById('chat-input') as HTMLInputElement;
const sendBtn = document.getElementById('send-btn');
const chatContainer = document.getElementById('chat-container');
const workspaceList = document.getElementById('workspace-list');
const sessionList = document.getElementById('session-list');
const newSessionBtn = document.getElementById('new-session-btn');
const workspaceIndicator = document.getElementById('current-workspace-name') as HTMLElement;
const openSettingsBtn = document.getElementById('open-settings-btn');
const closeSettingsBtn = document.getElementById('close-settings-btn');
const cancelSettingsBtn = document.getElementById('cancel-settings');
const settingsModal = document.getElementById('settings-modal');
const settingsForm = document.getElementById('settings-form') as HTMLFormElement;
const toolsList = document.getElementById('tools-list');

let currentAIResponseElement: any = null;
let currentAIResponseRaw = "";

// Configure Marked
// In marked v18+, highlight is handled differently.
// For now, let's keep it simple.
marked.setOptions({
    breaks: true
});

// --- Theme Logic ---
themeToggle?.addEventListener('click', () => {
    const currentTheme = body.getAttribute('data-theme');
    const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
    body.setAttribute('data-theme', newTheme);
    const icon = themeToggle.querySelector('i');
    if (icon) icon.className = newTheme === 'dark' ? 'fas fa-sun' : 'fas fa-moon';
});

// --- Data Fetching ---
async function loadWorkspaces() {
    try {
        const ws = await GetWorkspaces();
        if (workspaceList) {
            workspaceList.innerHTML = ws.map((w: any) => `
                <div class="nav-item staggered-item" data-id="${w.id}">
                    <i class="fas fa-folder-open" style="margin-right: 12px"></i>
                    ${w.name}
                </div>
            `).join('');
            
            workspaceList.querySelectorAll('.nav-item').forEach(item => {
                item.addEventListener('click', async () => {
                    const id = parseInt(item.getAttribute('data-id') || "0");
                    if (id) {
                        await SetWorkspace(id);
                        if (workspaceIndicator) workspaceIndicator.textContent = item.textContent?.trim() || "";
                        // Highlight active
                        workspaceList.querySelectorAll('.nav-item').forEach(i => i.classList.remove('active'));
                        item.classList.add('active');
                        
                        await loadSessions();
                        await loadHistory();
                    }
                });
            });
        }
    } catch (err) { console.error(err); }
}

async function loadHistory() {
    try {
        const history = await GetSessionHistory();
        if (chatContainer) {
            chatContainer.innerHTML = '';
            
            history.forEach((msg: any) => {
                if (msg.role === 'user' || msg.role === 'assistant') {
                    const type = msg.role === 'user' ? 'user' : 'ai';
                    const msgDiv = document.createElement('div');
                    msgDiv.className = `message ${type}`;
                    msgDiv.innerHTML = type === 'ai' ? (marked.parse(msg.content) as string) : msg.content;
                    chatContainer.appendChild(msgDiv);
                }
            });
            
            // Re-add thinking bubble at the end
            const newBubble = document.createElement('div');
            newBubble.id = 'thinking-bubble';
            newBubble.className = 'message ai thinking';
            newBubble.style.display = 'none';
            newBubble.innerHTML = `
                <div class="typing-dots">
                    <span></span><span></span><span></span>
                </div>
                <span style="margin-left: 8px">Smara sedang berpikir...</span>
            `;
            chatContainer.appendChild(newBubble);
            
            chatContainer.scrollTo({ top: chatContainer.scrollHeight, behavior: 'auto' });
        }
    } catch (err) { console.error(err); }
}

async function loadSessions() {
    try {
        const sessions = await GetSessions();
        if (sessionList) {
            sessionList.innerHTML = sessions.map((s: any) => `
                <div class="nav-item staggered-item" data-id="${s.id}" title="${s.name}">
                    ${s.name || 'Untitled Session'}
                </div>
            `).join('');
            
            sessionList.querySelectorAll('.nav-item').forEach(item => {
                item.addEventListener('click', async () => {
                    const id = item.getAttribute('data-id');
                    if (id) {
                        await SwitchSession(id);
                        await loadHistory();
                        // Highlight active
                        sessionList.querySelectorAll('.nav-item').forEach(i => i.classList.remove('active'));
                        item.classList.add('active');
                    }
                });
            });
        }
    } catch (err) { console.error(err); }
}

async function loadTools() {
    try {
        const tools = await GetTools();
        if (toolsList) {
            toolsList.innerHTML = tools.map((t: any) => `
                <div class="tool-badge" title="${t.description}">
                    <i class="fas fa-circle"></i> ${t.name}
                </div>
            `).join('');
        }
    } catch (err) { console.error(err); }
}

// --- Settings Logic ---
async function loadConfig() {
    try {
        const cfg = await GetConfig();
        const provider = document.getElementById('setting-provider') as HTMLSelectElement;
        const model = document.getElementById('setting-model') as HTMLInputElement;
        const apikey = document.getElementById('setting-apikey') as HTMLInputElement;
        const baseurl = document.getElementById('setting-baseurl') as HTMLInputElement;

        if (cfg) {
            provider.value = cfg.Provider || "ollama";
            
            // Map values based on current provider
            switch(cfg.Provider) {
                case 'openai':
                    model.value = cfg.OpenAIModel || "";
                    apikey.value = cfg.OpenAIAPIKey || "";
                    baseurl.value = cfg.OpenAIBaseURL || "";
                    break;
                case 'anthropic':
                    model.value = cfg.AnthropicModel || "";
                    apikey.value = cfg.AnthropicAPIKey || "";
                    baseurl.value = ""; 
                    break;
                case 'openrouter':
                    model.value = cfg.OpenRouterModel || "";
                    apikey.value = cfg.OpenRouterAPIKey || "";
                    baseurl.value = "";
                    break;
                case 'ollama':
                    model.value = cfg.Model || "";
                    apikey.value = "";
                    baseurl.value = cfg.OllamaHost || "";
                    break;
                case 'custom':
                    model.value = cfg.CustomModel || "";
                    apikey.value = cfg.CustomAPIKey || "";
                    baseurl.value = cfg.CustomBaseURL || "";
                    break;
            }
        }
    } catch (err) { console.error(err); }
}

openSettingsBtn?.addEventListener('click', () => {
    loadConfig();
    settingsModal?.classList.add('active');
});

const closeSettings = () => settingsModal?.classList.remove('active');
closeSettingsBtn?.addEventListener('click', closeSettings);
cancelSettingsBtn?.addEventListener('click', closeSettings);

settingsForm?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const provider = (document.getElementById('setting-provider') as HTMLSelectElement).value;
    const model = (document.getElementById('setting-model') as HTMLInputElement).value;
    const apikey = (document.getElementById('setting-apikey') as HTMLInputElement).value;
    const baseurl = (document.getElementById('setting-baseurl') as HTMLInputElement).value;

    try {
        const cfg = await GetConfig();
        cfg.Provider = provider;
        
        // Update specific fields based on chosen provider
        switch(provider) {
            case 'openai':
                cfg.OpenAIModel = model;
                cfg.OpenAIAPIKey = apikey;
                cfg.OpenAIBaseURL = baseurl;
                break;
            case 'anthropic':
                cfg.AnthropicModel = model;
                cfg.AnthropicAPIKey = apikey;
                break;
            case 'openrouter':
                cfg.OpenRouterModel = model;
                cfg.OpenRouterAPIKey = apikey;
                break;
            case 'ollama':
                cfg.Model = model;
                cfg.OllamaHost = baseurl;
                break;
            case 'custom':
                cfg.CustomModel = model;
                cfg.CustomAPIKey = apikey;
                cfg.CustomBaseURL = baseurl;
                break;
        }

        await UpdateConfig(cfg);
        closeSettings();
        alert("Konfigurasi berhasil disimpan!");
        loadTools(); 
    } catch (err) {
        alert("Gagal menyimpan konfigurasi: " + err);
    }
});

// --- Chat Logic ---
EventsOn("stream_chunk", (data: any) => {
    if (data.is_thinking) return;
    const activeBubble = document.getElementById('thinking-bubble');
    if (!currentAIResponseElement) {
        const msgDiv = document.createElement('div');
        msgDiv.className = `message ai`;
        chatContainer?.insertBefore(msgDiv, activeBubble);
        currentAIResponseElement = msgDiv;
        currentAIResponseRaw = "";
    }
    currentAIResponseRaw += data.chunk;
    currentAIResponseElement.innerHTML = marked.parse(currentAIResponseRaw) as string;
    chatContainer?.scrollTo({ top: chatContainer.scrollHeight, behavior: 'auto' });
});

function addMessage(text: string, type: 'user' | 'ai') {
    const activeBubble = document.getElementById('thinking-bubble');
    const msgDiv = document.createElement('div');
    msgDiv.className = `message ${type}`;
    msgDiv.innerHTML = type === 'ai' ? (marked.parse(text) as string) : text;
    chatContainer?.insertBefore(msgDiv, activeBubble);
    chatContainer?.scrollTo({ top: chatContainer.scrollHeight, behavior: 'smooth' });
}

async function handleSendMessage() {
    const text = chatInput.value.trim();
    if (!text) return;
    const activeBubble = document.getElementById('thinking-bubble');
    currentAIResponseElement = null;
    currentAIResponseRaw = "";
    addMessage(text, 'user');
    chatInput.value = '';
    if (activeBubble) activeBubble.style.display = 'flex';
    chatContainer?.scrollTo({ top: chatContainer.scrollHeight, behavior: 'smooth' });
    try {
        const finalResponse = await Ask(text);
        if (activeBubble) activeBubble.style.display = 'none';
        if (!currentAIResponseElement) addMessage(finalResponse, 'ai');
        else {
            currentAIResponseElement.innerHTML = marked.parse(finalResponse) as string;
        }
        loadSessions(); 
    } catch (err) {
        console.error(err);
        if (activeBubble) activeBubble.style.display = 'none';
        addMessage("Maaf, terjadi kesalahan.", 'ai');
    }
}

newSessionBtn?.addEventListener('click', async () => {
    const name = prompt("Nama Session Baru:", "Sesi Baru");
    if (name) {
        try {
            await CreateSession(name);
            if (chatContainer) {
                chatContainer.innerHTML = ''; 
                // Re-add bubble (from loadHistory logic)
                await loadHistory();
            }
            await loadSessions();
        } catch (err) {
            console.error(err);
        }
    }
});

sendBtn?.addEventListener('click', handleSendMessage);
chatInput?.addEventListener('keypress', (e) => { if (e.key === 'Enter') handleSendMessage(); });

// Init
loadWorkspaces();
loadSessions();
loadHistory();
loadTools();
