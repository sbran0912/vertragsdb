import htmx from 'htmx.org';

// Make htmx available globally
window.htmx = htmx;

// State management
const state = {
    user: null,
    token: null,
    currentContract: null,
    frameworkContracts: [],
};

// API helper
async function api(endpoint, options = {}) {
    const headers = {
        'Content-Type': 'application/json',
        ...options.headers,
    };

    if (state.token) {
        headers['Authorization'] = `Bearer ${state.token}`;
    }

    const response = await fetch(`/api${endpoint}`, {
        ...options,
        headers,
    });

    if (response.status === 401) {
        logout();
        return;
    }

    if (!response.ok && response.status !== 404) {
        const error = await response.text();
        throw new Error(error || 'Request failed');
    }

    if (response.status === 204) {
        return null;
    }

    return response.json();
}

// Auth functions
function saveAuth(token, user) {
    state.token = token;
    state.user = user;
    localStorage.setItem('token', token);
    localStorage.setItem('user', JSON.stringify(user));
}

function loadAuth() {
    const token = localStorage.getItem('token');
    const user = localStorage.getItem('user');
    if (token && user) {
        state.token = token;
        state.user = JSON.parse(user);
        return true;
    }
    return false;
}

function logout() {
    state.token = null;
    state.user = null;
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    showPage('login');
}

// Page navigation
function showPage(pageName) {
    document.querySelectorAll('.page').forEach(page => page.classList.add('hidden'));
    
    if (pageName === 'login') {
        document.getElementById('login-page').classList.remove('hidden');
    } else {
        document.getElementById('main-page').classList.remove('hidden');
        showContent(pageName);
    }
}

function showContent(contentName) {
    document.querySelectorAll('.content-page').forEach(page => page.classList.add('hidden'));
    document.querySelectorAll('.nav-link').forEach(link => link.classList.remove('active'));
    
    const page = document.getElementById(`${contentName}-page`);
    if (page) {
        page.classList.remove('hidden');
    }
    
    const link = document.querySelector(`[data-page="${contentName}"]`);
    if (link) {
        link.classList.add('active');
    }
    
    // Load data based on page
    if (contentName === 'contracts') {
        loadContracts();
    } else if (contentName === 'users') {
        loadUsers();
    } else if (contentName === 'settings') {
        loadCategoriesAdmin();
    }
}

function updateUIForRole() {
    const isAdmin = state.user?.role === 'admin';
    document.querySelectorAll('.admin-only').forEach(el => {
        el.style.display = isAdmin ? '' : 'none';
    });
    document.getElementById('user-info').textContent = 
        `${state.user.username} (${state.user.role === 'admin' ? 'Admin' : 'Viewer'})`;
}

// Format date
function formatDate(dateString) {
    if (!dateString) return '-';
    const date = new Date(dateString);
    return date.toLocaleDateString('de-DE');
}

function formatDateTime(dateString) {
    if (!dateString) return '-';
    const date = new Date(dateString);
    return date.toLocaleString('de-DE');
}

// Contracts
async function loadContracts(filters = {}) {
    try {
        const params = new URLSearchParams();
        if (filters.search) params.append('search', filters.search);
        if (filters.category) params.append('category', filters.category);
        if (filters.onlyValid) params.append('only_valid', 'true');
        
        const contracts = await api(`/contracts?${params}`);
        renderContracts(contracts);
    } catch (error) {
        console.error('Error loading contracts:', error);
    }
}

function renderContracts(contracts) {
    const container = document.getElementById('contracts-list');
    
    if (!contracts || contracts.length === 0) {
        container.innerHTML = `
            <div class="empty-state">
                <h3>Keine Verträge gefunden</h3>
                <p>Klicken Sie auf "Neuer Vertrag" um einen Vertrag anzulegen.</p>
            </div>
        `;
        return;
    }
    
    container.innerHTML = contracts.map(contract => {
        const isValid = !contract.is_terminated && 
            (!contract.valid_until || new Date(contract.valid_until) > new Date());
        const status = contract.is_terminated ? 'beendet' : (isValid ? 'gültig' : 'abgelaufen');
        const badgeClass = contract.is_terminated ? 'badge-danger' : (isValid ? 'badge-success' : 'badge-warning');
        
        return `
            <div class="contract-card" onclick="viewContract(${contract.id})">
                <div class="contract-header">
                    <div>
                        <div class="contract-title">${escapeHtml(contract.title)}</div>
                        <div class="contract-meta">
                            <span>Partner: ${escapeHtml(contract.partner)}</span>
                            <span>Kategorie: ${escapeHtml(contract.category)}</span>
                        </div>
                    </div>
                    <div class="contract-number">${escapeHtml(contract.contract_number)}</div>
                </div>
                <div class="contract-meta">
                    <span>Gültig ab: ${formatDate(contract.valid_from)}</span>
                    ${contract.valid_until ? `<span>Gültig bis: ${formatDate(contract.valid_until)}</span>` : ''}
                    <span class="badge ${badgeClass}">${status}</span>
                    ${contract.contract_type === 'framework' ? '<span class="badge badge-info">Rahmenvertrag</span>' : ''}
                </div>
            </div>
        `;
    }).join('');
}

async function viewContract(id) {
    try {
        const contract = await api(`/contracts/${id}`);
        state.currentContract = contract;
        renderContractDetail(contract);
        showContent('contract-detail');
    } catch (error) {
        console.error('Error loading contract:', error);
    }
}

async function renderContractDetail(contract) {
    const container = document.getElementById('contract-detail-content');
    
    // Load documents
    const documents = await api(`/contracts/${contract.id}/documents`);
    
    // Load framework contract if exists
    let frameworkInfo = '';
    if (contract.framework_contract_id) {
        try {
            const framework = await api(`/contracts/${contract.framework_contract_id}`);
            frameworkInfo = `
                <div class="detail-item">
                    <div class="detail-label">Rahmenvertrag</div>
                    <div class="detail-value">
                        ${escapeHtml(framework.contract_number)} - ${escapeHtml(framework.title)}
                    </div>
                </div>
            `;
        } catch (e) {
            console.error('Error loading framework contract:', e);
        }
    }
    
    const isValid = !contract.is_terminated && 
        (!contract.valid_until || new Date(contract.valid_until) > new Date());
    const status = contract.is_terminated ? 'Beendet' : (isValid ? 'Gültig' : 'Abgelaufen');
    const badgeClass = contract.is_terminated ? 'badge-danger' : (isValid ? 'badge-success' : 'badge-warning');
    
    container.innerHTML = `
        <div class="detail-section">
            <h3>Allgemeine Informationen</h3>
            <div class="detail-grid">
                <div class="detail-item">
                    <div class="detail-label">Vertragsnummer</div>
                    <div class="detail-value">${escapeHtml(contract.contract_number)}</div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Status</div>
                    <div class="detail-value"><span class="badge ${badgeClass}">${status}</span></div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Titel</div>
                    <div class="detail-value">${escapeHtml(contract.title)}</div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Vertragspartner</div>
                    <div class="detail-value">${escapeHtml(contract.partner)}</div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Kategorie</div>
                    <div class="detail-value">${escapeHtml(contract.category)}</div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Vertragstyp</div>
                    <div class="detail-value">${contract.contract_type === 'framework' ? 'Rahmenvertrag' : 'Einzelvertrag'}</div>
                </div>
                ${frameworkInfo}
            </div>
        </div>

        <div class="detail-section">
            <h3>Laufzeit und Fristen</h3>
            <div class="detail-grid">
                <div class="detail-item">
                    <div class="detail-label">Gültig ab</div>
                    <div class="detail-value">${formatDate(contract.valid_from)}</div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Gültig bis</div>
                    <div class="detail-value">${formatDate(contract.valid_until)}</div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Kündigungsfrist</div>
                    <div class="detail-value">${contract.notice_period != null ? contract.notice_period + ' Monate' : '-'}</div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Mindestlaufzeit bis</div>
                    <div class="detail-value">${formatDate(contract.minimum_term)}</div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Laufzeit</div>
                    <div class="detail-value">${contract.term_months != null ? contract.term_months + ' Monate' : '-'}</div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Kündigungstermin</div>
                    <div class="detail-value">${formatDate(contract.cancellation_date)}</div>
                </div>
                <div class="detail-item">
                    <div class="detail-label">Kündigungsvornahme</div>
                    <div class="detail-value"><strong>${formatDate(contract.cancellation_action_date)}</strong></div>
                </div>
                ${contract.is_terminated ? `
                <div class="detail-item">
                    <div class="detail-label">Beendet am</div>
                    <div class="detail-value">${formatDateTime(contract.terminated_at)}</div>
                </div>
                ` : ''}
            </div>
        </div>

        <div class="detail-section">
            <h3>Vertragsinhalt</h3>
            <div class="detail-item">
                <div class="detail-value">${escapeHtml(contract.content) || '-'}</div>
            </div>
        </div>

        <div class="detail-section">
            <h3>Vertragskonditionen</h3>
            <div class="detail-item">
                <div class="detail-value">${escapeHtml(contract.conditions) || '-'}</div>
            </div>
        </div>

        <div class="detail-section">
            <h3>Dokumente</h3>
            ${state.user?.role === 'admin' ? `
            <div class="upload-area">
                <input type="file" id="document-upload" accept=".pdf" />
                <button onclick="uploadDocument()" class="btn btn-primary">Dokument hochladen</button>
            </div>
            ` : ''}
            ${documents && documents.length > 0 ? `
                <ul class="document-list">
                    ${documents.map(doc => `
                        <li class="document-item">
                            <div>
                                <div class="document-name">${escapeHtml(doc.filename)}</div>
                                <div class="document-date">Hochgeladen: ${formatDateTime(doc.uploaded_at)}</div>
                            </div>
                            <button onclick="downloadDocument(${doc.id})" class="btn btn-secondary">Download</button>
                        </li>
                    `).join('')}
                </ul>
            ` : '<p>Keine Dokumente vorhanden</p>'}
        </div>
    `;
}

window.viewContract = viewContract;

async function uploadDocument() {
    const fileInput = document.getElementById('document-upload');
    const file = fileInput.files[0];
    
    if (!file) {
        alert('Bitte wählen Sie eine Datei aus');
        return;
    }
    
    if (!file.name.toLowerCase().endsWith('.pdf')) {
        alert('Bitte wählen Sie eine PDF-Datei aus');
        return;
    }
    
    const formData = new FormData();
    formData.append('document', file);
    
    try {
        const response = await fetch(`/api/contracts/${state.currentContract.id}/documents`, {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${state.token}`,
            },
            body: formData,
        });
        
        if (response.ok) {
            alert('Dokument erfolgreich hochgeladen');
            viewContract(state.currentContract.id);
        } else {
            alert('Fehler beim Hochladen');
        }
    } catch (error) {
        console.error('Error uploading document:', error);
        alert('Fehler beim Hochladen');
    }
}

window.uploadDocument = uploadDocument;

async function downloadDocument(docId) {
    try {
        const response = await fetch(`/api/documents/${docId}/download`, {
            headers: { 'Authorization': `Bearer ${state.token}` },
        });
        if (!response.ok) throw new Error('Download fehlgeschlagen');
        const blob = await response.blob();
        const disposition = response.headers.get('Content-Disposition') || '';
        const match = disposition.match(/filename=(.+)/);
        const filename = match ? match[1] : 'dokument.pdf';
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        a.click();
        URL.revokeObjectURL(url);
    } catch (error) {
        console.error('Error downloading document:', error);
        alert('Fehler beim Herunterladen des Dokuments');
    }
}

window.downloadDocument = downloadDocument;

async function terminateContract() {
    if (!confirm('Möchten Sie diesen Vertrag wirklich beenden? Diese Aktion kann nicht rückgängig gemacht werden.')) {
        return;
    }
    
    try {
        await api(`/contracts/${state.currentContract.id}/terminate`, { method: 'POST' });
        alert('Vertrag wurde beendet');
        showContent('contracts');
    } catch (error) {
        console.error('Error terminating contract:', error);
        alert('Fehler beim Beenden des Vertrags');
    }
}

window.terminateContract = terminateContract;

// Contract form
async function showContractForm(contractId = null) {
    const formTitle = document.getElementById('form-title');
    const form = document.getElementById('contract-form');
    
    // Load framework contracts for dropdown
    try {
        const contracts = await api('/contracts');
        state.frameworkContracts = contracts.filter(c => c.contract_type === 'framework');
        updateFrameworkDropdown();
    } catch (error) {
        console.error('Error loading framework contracts:', error);
    }
    
    if (contractId) {
        formTitle.textContent = 'Vertrag bearbeiten';
        const contract = await api(`/contracts/${contractId}`);
        
        form.elements['contract_type'].value = contract.contract_type;
        form.elements['title'].value = contract.title;
        form.elements['partner'].value = contract.partner;
        form.elements['category'].value = contract.category;
        form.elements['valid_from'].value = contract.valid_from.split('T')[0];
        if (contract.valid_until) {
            form.elements['valid_until'].value = contract.valid_until.split('T')[0];
        }
        form.elements['content'].value = contract.content || '';
        form.elements['conditions'].value = contract.conditions || '';
        form.elements['notice_period'].value = contract.notice_period != null ? contract.notice_period : '';
        form.elements['minimum_term'].value = contract.minimum_term ? contract.minimum_term.split('T')[0] : '';
        form.elements['term_months'].value = contract.term_months != null ? contract.term_months : '';
        if (contract.framework_contract_id) {
            form.elements['framework_contract_id'].value = contract.framework_contract_id;
        }
        
        form.dataset.contractId = contractId;
    } else {
        formTitle.textContent = 'Neuer Vertrag';
        form.reset();
        delete form.dataset.contractId;
    }
    
    updateContractTypeFields();
    showContent('contract-form');
}

function updateFrameworkDropdown() {
    const select = document.getElementById('framework-contract');
    select.innerHTML = '<option value="">Keiner</option>' +
        state.frameworkContracts.map(c => 
            `<option value="${c.id}">${c.contract_number} - ${escapeHtml(c.title)}</option>`
        ).join('');
}

function updateContractTypeFields() {
    const contractType = document.getElementById('contract-type').value;
    const frameworkGroup = document.getElementById('framework-group');
    
    if (contractType === 'individual') {
        frameworkGroup.style.display = '';
    } else {
        frameworkGroup.style.display = 'none';
        document.getElementById('framework-contract').value = '';
    }
}

async function saveContract(formData) {
    const contractId = document.getElementById('contract-form').dataset.contractId;
    
    const data = {
        contract_type: formData.get('contract_type'),
        title: formData.get('title'),
        partner: formData.get('partner'),
        category: formData.get('category'),
        valid_from: new Date(formData.get('valid_from')).toISOString(),
        valid_until: formData.get('valid_until') ? new Date(formData.get('valid_until')).toISOString() : null,
        content: formData.get('content'),
        conditions: formData.get('conditions'),
        notice_period: formData.get('notice_period') ? parseInt(formData.get('notice_period')) : null,
        minimum_term: formData.get('minimum_term') ? new Date(formData.get('minimum_term')).toISOString() : null,
        term_months: formData.get('term_months') ? parseInt(formData.get('term_months')) : null,
        framework_contract_id: formData.get('framework_contract_id') ? parseInt(formData.get('framework_contract_id')) : null,
    };
    
    try {
        if (contractId) {
            await api(`/contracts/${contractId}`, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        } else {
            await api('/contracts', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        }
        
        showContent('contracts');
    } catch (error) {
        console.error('Error saving contract:', error);
        alert('Fehler beim Speichern des Vertrags');
    }
}

// Users
async function loadUsers() {
    try {
        const users = await api('/users');
        renderUsers(users);
    } catch (error) {
        console.error('Error loading users:', error);
    }
}

function renderUsers(users) {
    const container = document.getElementById('users-list');
    const isAdmin = state.user?.role === 'admin';

    container.innerHTML = `
        <table class="table">
            <thead>
                <tr>
                    <th>Benutzername</th>
                    <th>Rolle</th>
                    ${isAdmin ? '<th>Aktionen</th>' : ''}
                </tr>
            </thead>
            <tbody>
                ${users.map(user => `
                    <tr>
                        <td>${escapeHtml(user.username)}</td>
                        <td>${user.role === 'admin' ? 'Admin' : 'Viewer'}</td>
                        ${isAdmin ? `
                        <td>
                            <button onclick="editUser(${user.id})" class="btn btn-secondary" style="margin-right:4px">Bearbeiten</button>
                            <button onclick="deleteUser(${user.id}, '${escapeHtml(user.username)}')" class="btn btn-danger">Löschen</button>
                        </td>` : ''}
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;
}

function openUserModal({ title, submitLabel, userId, username, role, passwordRequired }) {
    const form = document.getElementById('user-form');
    form.reset();
    form.dataset.userId = userId || '';

    document.getElementById('user-modal-title').textContent = title;
    document.getElementById('user-submit-btn').textContent = submitLabel;

    if (username) document.getElementById('new-username').value = username;
    if (role) document.getElementById('role').value = role;

    const passwordInput = document.getElementById('new-password');
    const passwordLabel = document.getElementById('new-password-label');
    if (passwordRequired) {
        passwordInput.required = true;
        passwordInput.placeholder = '';
        passwordLabel.textContent = 'Passwort *';
    } else {
        passwordInput.required = false;
        passwordInput.placeholder = 'Leer lassen, um Passwort beizubehalten';
        passwordLabel.textContent = 'Passwort';
    }

    document.getElementById('user-modal').classList.remove('hidden');
}

async function editUser(userId) {
    try {
        const users = await api('/users');
        const user = users.find(u => u.id === userId);
        if (!user) return;
        openUserModal({
            title: 'Benutzer bearbeiten',
            submitLabel: 'Speichern',
            userId: user.id,
            username: user.username,
            role: user.role,
            passwordRequired: false,
        });
    } catch (error) {
        console.error('Error loading user:', error);
    }
}

window.editUser = editUser;

async function deleteUser(userId, username) {
    if (!confirm(`Benutzer „${username}" wirklich löschen?`)) return;
    try {
        await api(`/users/${userId}`, { method: 'DELETE' });
        loadUsers();
    } catch (error) {
        console.error('Error deleting user:', error);
        alert('Fehler beim Löschen: ' + error.message);
    }
}

window.deleteUser = deleteUser;

async function saveUser(formData) {
    const form = document.getElementById('user-form');
    const userId = form.dataset.userId;
    const data = {
        username: formData.get('username'),
        password: formData.get('password'),
        role: formData.get('role'),
    };

    try {
        if (userId) {
            await api(`/users/${userId}`, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        } else {
            await api('/users', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        }
        document.getElementById('user-modal').classList.add('hidden');
        loadUsers();
    } catch (error) {
        console.error('Error saving user:', error);
        alert('Fehler beim Speichern: ' + error.message);
    }
}

// Categories
async function loadCategories() {
    try {
        const categories = await api('/categories');
        populateCategoryDropdowns(categories || []);
        return categories || [];
    } catch (error) {
        console.error('Error loading categories:', error);
        return [];
    }
}

function populateCategoryDropdowns(categories) {
    const filterSelect = document.getElementById('category-filter');
    const currentFilterValue = filterSelect.value;
    filterSelect.innerHTML = '<option value="">Alle Kategorien</option>' +
        categories.map(c => `<option value="${escapeHtml(c.name)}">${escapeHtml(c.name)}</option>`).join('');
    filterSelect.value = currentFilterValue;

    const formSelect = document.getElementById('category');
    const currentFormValue = formSelect.value;
    formSelect.innerHTML = '<option value="">Bitte wählen...</option>' +
        categories.map(c => `<option value="${escapeHtml(c.name)}">${escapeHtml(c.name)}</option>`).join('');
    formSelect.value = currentFormValue;
}

async function loadCategoriesAdmin() {
    try {
        const categories = await api('/categories');
        renderCategoriesAdmin(categories || []);
    } catch (error) {
        console.error('Error loading categories:', error);
    }
}

function renderCategoriesAdmin(categories) {
    const container = document.getElementById('categories-list');

    if (!categories || categories.length === 0) {
        container.innerHTML = '<p>Keine Kategorien vorhanden</p>';
        return;
    }

    container.innerHTML = `
        <table class="table">
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Aktionen</th>
                </tr>
            </thead>
            <tbody>
                ${categories.map(cat => `
                    <tr>
                        <td>${escapeHtml(cat.name)}</td>
                        <td>
                            <button onclick="editCategory(${cat.id})" class="btn btn-secondary" style="margin-right:4px">Bearbeiten</button>
                            <button onclick="deleteCategory(${cat.id}, '${escapeHtml(cat.name)}')" class="btn btn-danger">Löschen</button>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;
}

function openCategoryModal({ title, submitLabel, categoryId, name }) {
    const form = document.getElementById('category-form');
    form.reset();
    form.dataset.categoryId = categoryId || '';

    document.getElementById('category-modal-title').textContent = title;
    document.getElementById('category-submit-btn').textContent = submitLabel;

    if (name) document.getElementById('category-name').value = name;

    document.getElementById('category-modal').classList.remove('hidden');
}

async function editCategory(categoryId) {
    try {
        const categories = await api('/categories');
        const cat = categories.find(c => c.id === categoryId);
        if (!cat) return;
        openCategoryModal({
            title: 'Kategorie bearbeiten',
            submitLabel: 'Speichern',
            categoryId: cat.id,
            name: cat.name,
        });
    } catch (error) {
        console.error('Error loading category:', error);
    }
}
window.editCategory = editCategory;

async function deleteCategory(categoryId, name) {
    if (!confirm(`Kategorie "${name}" wirklich löschen?`)) return;
    try {
        await api(`/categories/${categoryId}`, { method: 'DELETE' });
        loadCategoriesAdmin();
        loadCategories();
    } catch (error) {
        console.error('Error deleting category:', error);
        alert('Fehler beim Löschen: ' + error.message);
    }
}
window.deleteCategory = deleteCategory;

async function saveCategory(formData) {
    const form = document.getElementById('category-form');
    const categoryId = form.dataset.categoryId;
    const data = { name: formData.get('name') };

    try {
        if (categoryId) {
            await api(`/categories/${categoryId}`, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        } else {
            await api('/categories', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        }
        document.getElementById('category-modal').classList.add('hidden');
        loadCategoriesAdmin();
        loadCategories();
    } catch (error) {
        console.error('Error saving category:', error);
        alert('Fehler beim Speichern: ' + error.message);
    }
}

// Reports
async function showValidContracts() {
    try {
        const contracts = await api('/contracts?only_valid=true');
        const container = document.getElementById('valid-contracts-list');
        
        if (!contracts || contracts.length === 0) {
            container.innerHTML = '<p>Keine gültigen Verträge gefunden</p>';
            return;
        }
        
        container.innerHTML = `
            <table class="table">
                <thead>
                    <tr>
                        <th>Vertragsnummer</th>
                        <th>Titel</th>
                        <th>Partner</th>
                        <th>Kategorie</th>
                        <th>Gültig bis</th>
                    </tr>
                </thead>
                <tbody>
                    ${contracts.map(c => `
                        <tr onclick="viewContract(${c.id})" style="cursor: pointer;">
                            <td>${escapeHtml(c.contract_number)}</td>
                            <td>${escapeHtml(c.title)}</td>
                            <td>${escapeHtml(c.partner)}</td>
                            <td>${escapeHtml(c.category)}</td>
                            <td>${formatDate(c.valid_until)}</td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
    } catch (error) {
        console.error('Error loading valid contracts:', error);
    }
}

window.showValidContracts = showValidContracts;

async function showExpiringContracts() {
    try {
        const days = document.getElementById('warning-days').value || 90;
        const contracts = await api(`/reports/expiring?days=${days}`);
        const container = document.getElementById('expiring-contracts-list');

        if (!contracts || contracts.length === 0) {
            container.innerHTML = '<p>Keine Verträge mit ablaufender Kündigungsfrist gefunden. Wurden die Kündigungstermine schon berechnet?</p>';
            return;
        }

        container.innerHTML = `
            <table class="table">
                <thead>
                    <tr>
                        <th>Vertragsnummer</th>
                        <th>Titel</th>
                        <th>Partner</th>
                        <th>Kündigungstermin</th>
                        <th>Kündigungsvornahme</th>
                    </tr>
                </thead>
                <tbody>
                    ${contracts.map(c => `
                        <tr onclick="viewContract(${c.id})" style="cursor: pointer;">
                            <td>${escapeHtml(c.contract_number)}</td>
                            <td>${escapeHtml(c.title)}</td>
                            <td>${escapeHtml(c.partner)}</td>
                            <td>${formatDate(c.cancellation_date)}</td>
                            <td><strong>${formatDate(c.cancellation_action_date)}</strong></td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
    } catch (error) {
        console.error('Error loading expiring contracts:', error);
    }
}

window.showExpiringContracts = showExpiringContracts;

// Utility
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Event listeners
document.addEventListener('DOMContentLoaded', () => {
    // Login form
    document.getElementById('login-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const formData = new FormData(e.target);
        
        try {
            const response = await api('/login', {
                method: 'POST',
                body: JSON.stringify({
                    username: formData.get('username'),
                    password: formData.get('password'),
                }),
            });
            
            saveAuth(response.token, response.user);
            updateUIForRole();
            loadCategories();
            showPage('main');
            showContent('contracts');
        } catch (error) {
            document.getElementById('login-error').textContent = 'Anmeldung fehlgeschlagen';
        }
    });
    
    // Logout
    document.getElementById('logout-btn').addEventListener('click', logout);
    
    // Navigation
    document.querySelectorAll('.nav-link').forEach(link => {
        link.addEventListener('click', (e) => {
            e.preventDefault();
            const page = e.target.dataset.page;
            showContent(page);
        });
    });
    
    // Contract buttons
    document.getElementById('new-contract-btn').addEventListener('click', () => showContractForm());
    document.getElementById('back-to-contracts').addEventListener('click', () => showContent('contracts'));
    document.getElementById('edit-contract-btn').addEventListener('click', () => {
        if (state.currentContract) {
            showContractForm(state.currentContract.id);
        }
    });
    document.getElementById('terminate-contract-btn').addEventListener('click', terminateContract);
    
    // Contract form
    document.getElementById('contract-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const formData = new FormData(e.target);
        await saveContract(formData);
    });
    
    document.getElementById('cancel-form-btn').addEventListener('click', () => showContent('contracts'));
    document.getElementById('cancel-form-btn-2').addEventListener('click', () => showContent('contracts'));
    document.getElementById('contract-type').addEventListener('change', updateContractTypeFields);
    
    // Search
    document.getElementById('search-btn').addEventListener('click', () => {
        loadContracts({
            search: document.getElementById('search-input').value,
            category: document.getElementById('category-filter').value,
            onlyValid: document.getElementById('only-valid-filter').checked,
        });
    });
    
    // User management
    document.getElementById('new-user-btn').addEventListener('click', () => {
        openUserModal({
            title: 'Neuer Benutzer',
            submitLabel: 'Erstellen',
            passwordRequired: true,
        });
    });

    document.getElementById('cancel-user-btn').addEventListener('click', () => {
        document.getElementById('user-modal').classList.add('hidden');
    });

    document.getElementById('user-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const formData = new FormData(e.target);
        await saveUser(formData);
    });
    
    // Category management
    document.getElementById('new-category-btn').addEventListener('click', () => {
        openCategoryModal({
            title: 'Neue Kategorie',
            submitLabel: 'Erstellen',
        });
    });

    document.getElementById('cancel-category-btn').addEventListener('click', () => {
        document.getElementById('category-modal').classList.add('hidden');
    });

    document.getElementById('category-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const formData = new FormData(e.target);
        await saveCategory(formData);
    });
    
    // Reports
    document.getElementById('show-valid-contracts').addEventListener('click', showValidContracts);
    document.getElementById('show-expiring-contracts').addEventListener('click', showExpiringContracts);
    document.getElementById('calculate-dates-btn').addEventListener('click', async () => {
        try {
            const result = await api('/contracts/calculate-dates', { method: 'POST' });
            document.getElementById('calculate-dates-result').textContent = result.message;
        } catch (error) {
            console.error('Error calculating dates:', error);
            alert('Fehler bei der Berechnung: ' + error.message);
        }
    });
    
    // Check if already logged in
    if (loadAuth()) {
        updateUIForRole();
        loadCategories();
        showPage('main');
        showContent('contracts');
    } else {
        showPage('login');
    }
});
