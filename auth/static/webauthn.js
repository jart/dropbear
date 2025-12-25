function base64urlToBuffer(base64url) {
    const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
    const pad = base64.length % 4 === 0 ? '' : '='.repeat(4 - (base64.length % 4));
    const binary = atob(base64 + pad);
    return Uint8Array.from(binary, c => c.charCodeAt(0));
}

function bufferToBase64url(buffer) {
    const binary = String.fromCharCode(...new Uint8Array(buffer));
    return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

async function webauthnRegister(inviteToken) {
    const name = document.getElementById('name').value.trim();
    if (!name) {
        document.getElementById('error').textContent = 'Please enter your name';
        return;
    }

    const btn = document.getElementById('register');
    btn.disabled = true;
    document.getElementById('error').textContent = '';

    try {
        const beginResp = await fetch('/auth/register/begin?token=' + encodeURIComponent(inviteToken), {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({name})
        });
        if (!beginResp.ok) {
            throw new Error(await beginResp.text());
        }
        const options = await beginResp.json();

        options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);
        options.publicKey.user.id = base64urlToBuffer(options.publicKey.user.id);
        if (options.publicKey.excludeCredentials) {
            options.publicKey.excludeCredentials = options.publicKey.excludeCredentials.map(c => ({
                ...c,
                id: base64urlToBuffer(c.id)
            }));
        }

        const credential = await navigator.credentials.create(options);

        const response = {
            id: credential.id,
            rawId: bufferToBase64url(credential.rawId),
            type: credential.type,
            response: {
                clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
                attestationObject: bufferToBase64url(credential.response.attestationObject)
            }
        };

        const finishResp = await fetch('/auth/register/finish?token=' + encodeURIComponent(inviteToken), {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({name, response})
        });
        if (!finishResp.ok) {
            throw new Error(await finishResp.text());
        }

        window.location.href = '/';
    } catch (e) {
        document.getElementById('error').textContent = e.message || 'Registration failed';
        btn.disabled = false;
    }
}

async function webauthnLogin() {
    const name = document.getElementById('name').value.trim();
    if (!name) {
        document.getElementById('error').textContent = 'Please enter your name';
        return;
    }

    const btn = document.getElementById('login');
    btn.disabled = true;
    document.getElementById('error').textContent = '';

    try {
        const beginResp = await fetch('/auth/login/begin', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({name})
        });
        if (!beginResp.ok) {
            throw new Error(await beginResp.text());
        }
        const options = await beginResp.json();

        options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);
        if (options.publicKey.allowCredentials) {
            options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(c => ({
                ...c,
                id: base64urlToBuffer(c.id)
            }));
        }

        const credential = await navigator.credentials.get(options);

        const response = {
            id: credential.id,
            rawId: bufferToBase64url(credential.rawId),
            type: credential.type,
            response: {
                clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
                authenticatorData: bufferToBase64url(credential.response.authenticatorData),
                signature: bufferToBase64url(credential.response.signature),
                userHandle: credential.response.userHandle ? bufferToBase64url(credential.response.userHandle) : null
            }
        };

        const finishResp = await fetch('/auth/login/finish', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({name, response})
        });
        if (!finishResp.ok) {
            throw new Error(await finishResp.text());
        }

        window.location.href = '/';
    } catch (e) {
        document.getElementById('error').textContent = e.message || 'Login failed';
        btn.disabled = false;
    }
}
