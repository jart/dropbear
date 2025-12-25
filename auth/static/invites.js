async function loadInvites() {
    const resp = await fetch('/auth/invites/list');
    const invites = await resp.json();
    const container = document.getElementById('invites');
    if (!invites || invites.length === 0) {
        container.innerHTML = '<p class="info">No invites yet</p>';
        return;
    }
    container.innerHTML = invites.map(inv => {
        const url = window.location.origin + '/register?token=' + encodeURIComponent(inv.Token);
        if (inv.UsedBy) {
            return '<div class="invite invite-used">Used</div>';
        }
        return '<div class="invite"><span class="invite-link">' + url + '</span><button class="invite-copy" onclick="navigator.clipboard.writeText(\'' + url + '\')">Copy</button></div>';
    }).join('');
}

document.getElementById('create').onclick = async () => {
    await fetch('/auth/invites/create', {method: 'POST'});
    loadInvites();
};

loadInvites();
