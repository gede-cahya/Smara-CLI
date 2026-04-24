document.addEventListener('DOMContentLoaded', () => {
  const themeToggle = document.getElementById('theme-toggle');
  const body = document.body;
  const icon = themeToggle.querySelector('i');
  const chatInput = document.getElementById('chat-input');
  const sendBtn = document.querySelector('.send-btn');
  const chatContainer = document.getElementById('chat-container');

  // 1. Theme Toggle Logic
  themeToggle.addEventListener('click', () => {
    const isDark = body.getAttribute('data-theme') === 'dark';
    const newTheme = isDark ? 'light' : 'dark';
    
    body.setAttribute('data-theme', newTheme);
    
    // Update Icon
    icon.className = isDark ? 'fas fa-moon' : 'fas fa-sun';
    
    // Subtle button animation
    themeToggle.style.transform = 'rotate(360deg)';
    setTimeout(() => themeToggle.style.transform = 'rotate(0deg)', 400);
  });

  // 2. Chat Interaction Mockup
  function addMessage(text, role) {
    const msg = document.createElement('div');
    msg.className = `message ${role}`;
    msg.textContent = text;
    chatContainer.appendChild(msg);
    chatContainer.scrollTop = chatContainer.scrollHeight;
  }

  function handleSend() {
    const text = chatInput.value.trim();
    if (!text) return;

    // Add User Message
    addMessage(text, 'user');
    chatInput.value = '';

    // Simulate AI Thinking
    const thinking = document.getElementById('thinking-bubble');
    thinking.style.display = 'flex';
    chatContainer.appendChild(thinking); // Move to bottom

    setTimeout(() => {
      thinking.style.display = 'none';
      addMessage("Pesan Anda telah diterima. Saya sedang memproses permintaan tersebut dalam workspace yang aktif.", "ai");
    }, 1500);
  }

  sendBtn.addEventListener('click', handleSend);
  chatInput.addEventListener('keypress', (e) => {
    if (e.key === 'Enter') handleSend();
  });

  // 3. Initial Animations Refresh
  // (Adding staggering effect via JS if needed, but CSS handles initial load)
});
