import { apiFetch as request } from "./api.js";
import { changeIdentity } from "./browser-context.js";
import { escapeHTML } from "./utils.js";
import { renderQRCode } from "./qr.js";

async function apiFetch(path, options = {}) { return (await request(path, options)).data; }
async function householdTransition(path, options) {
  const result = await changeIdentity(async () => {
    const {response,data}=await request(path,{...options,allowContextChange:true});
    return {ok:response.ok,data};
  });
  return {...result.data, ok:result.ok, user:result.user, identity:result.identity};
}

export async function listHouseholds() {
  return apiFetch("/api/households");
}

export async function activateHousehold(id) {
  return householdTransition(`/api/households/${id}/activate`, { method: "POST" });
}

export async function loadHousehold() {
  try {
    return await apiFetch("/api/household");
  } catch (err) {
    if (err.status === 404) return {};
    if (err.status === 403 && err.data?.status === "access_suspended") return { suspended: err.data };
    throw err;
  }
}

export async function createHousehold(name, initials) {
  return householdTransition("/api/household", {
    method: "POST",
    body: JSON.stringify({ name, initials: initials || "" }),
  });
}

export async function updateHousehold(name, initials) {
  return apiFetch("/api/household", {
    method: "PATCH",
    body: JSON.stringify({ name, initials: initials || "" }),
  });
}

export async function createInvite() {
  return apiFetch("/api/household/invites", { method: "POST" });
}

export async function deleteInvite(id) {
  return apiFetch(`/api/household/invites/${id}`, { method: "DELETE" });
}

// What an invite link shows before sign-in: the household's name and who sent
// it. Needs no session; a dead code is a 404.
export async function loadInvitePreview(code) {
  return apiFetch(`/api/household/join/preview?code=${encodeURIComponent(code)}`);
}

export async function joinHousehold(inviteCode) {
  return householdTransition("/api/household/join", {
    method: "POST",
    body: JSON.stringify({ inviteCode }),
  });
}

export async function removeMember(userId) {
  return apiFetch(`/api/household/members/${userId}`, { method: "DELETE" });
}

export async function updateMemberRole(userId, role) {
  return apiFetch(`/api/household/members/${userId}`, {
    method: "PATCH",
    body: JSON.stringify({ role }),
  });
}

export async function setMemberAccess(userId, suspended) {
  return apiFetch(`/api/household/members/${userId}/access`, {
    method: "PATCH",
    body: JSON.stringify({ suspended }),
  });
}

export async function leaveHousehold() {
  return householdTransition("/api/household/leave", { method: "POST" });
}

export async function transferOwnership(newOwnerId) {
  return apiFetch("/api/household/transfer", {
    method: "POST",
    body: JSON.stringify({ newOwnerId: newOwnerId }),
  });
}

const AUTH_CHOICES = `<button type="button" class="btn btn-primary btn-block" data-action="show-register">Create Account</button>
    <div class="auth-divider">or</div>
    <button type="button" class="btn btn-secondary btn-block" data-action="show-login">Sign In</button>`;

// The page an invite link opens for someone signed out. `invite` is the
// preview's state: loading, ready (household, inviter), invalid or unavailable.
export function renderJoinView(invite) {
  if (invite?.status === "invalid") {
    return `<div class="auth-card invite-card">
    <h1 class="auth-title">This invite link isn't valid any more</h1>
    <p class="text-center text-secondary mb-3">Ask whoever sent it for a new one. You can still create an account and set up your own household.</p>
    ${AUTH_CHOICES}
  </div>`;
  }
  if (invite?.status !== "ready") {
    return `<div class="auth-card invite-card"${invite?.status === "loading" ? ' aria-busy="true"' : ""}>
    <h1 class="auth-title">You're Invited!</h1>
    <p class="text-center text-secondary mb-3">Create an account or sign in to join this household on Nabu.</p>
    ${AUTH_CHOICES}
  </div>`;
  }
  const { household, inviter } = invite;
  const name = inviter?.displayName || "";
  const color = /^#[0-9A-Fa-f]{6}$/.test(inviter?.avatarColor || "") ? inviter.avatarColor : "#19323C";
  const badge = name
    ? `<span class="invite-avatar" style="background:${escapeHTML(color)}" aria-hidden="true">${escapeHTML(Array.from(name)[0].toUpperCase())}</span>`
    : household?.initials ? `<span class="invite-avatar invite-avatar--household" aria-hidden="true">${escapeHTML(household.initials)}</span>` : "";
  return `<div class="auth-card invite-card">
    ${badge}
    <p class="invite-lead">${name ? `${escapeHTML(name)} invited you to join` : "You're invited to join"}</p>
    <h1 class="auth-title">${escapeHTML(household?.name || "")}</h1>
    <p class="text-center text-secondary mb-3">Nabu is one shared log for your home: feeds, meds, walks and chores. Everyone sees what's been done. Create an account or sign in to join.</p>
    ${AUTH_CHOICES}
  </div>`;
}

// A QR code for an invite link, for someone in the room to scan.
export function renderInviteQR(link) {
  return `<div class="invite-qr">${renderQRCode(link, "QR code for the invite link")}</div>`;
}

export function renderHouseholdView(household, members, invites, currentUser, { showInviteQR = false } = {}) {
  if (!household) {
    return `<div class="card mt-3">
      <h3>Household</h3>
      <div class="empty-state">
        <p class="mt-2">You're not part of a household yet.</p>
        <a class="btn btn-primary btn-block mt-3" href="/today" data-nav="home">Set up a household</a>
        <div class="auth-divider">or</div>
        <form id="join-household-form" data-action="join-household">
          <div class="form-group">
            <label class="form-label" for="invite-code">Invite Code</label>
            <input id="invite-code" type="text" name="inviteCode" required placeholder="ABCDEF">
          </div>
          <button type="submit" class="btn btn-secondary btn-block">Join Household</button>
        </form>
      </div>
    </div>`;
  }
  const isOwner = currentUser && currentUser.role === 'owner';
  const isAdmin = currentUser && currentUser.role === 'admin';
  const ownerCount = (members || []).filter(m => m.role === 'owner').length;

  const chevronSVG = `<svg class="member-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><polyline points="6 9 12 15 18 9"></polyline></svg>`;

  const memberList = (members || []).map(m => {
    const initial = (m.displayName || m.email || '?')[0].toUpperCase();
    const label = m.displayName || m.email || 'Unknown';
    const isSelf = m.userId === currentUser.id;
    const isLastOwner = m.role === 'owner' && ownerCount <= 1;
    // Owners manage everyone but owners; admins may only disable plain members.
    const canToggleAccess = !isSelf && m.role !== 'owner' && (isOwner || (isAdmin && m.role === 'member'));

    let detailsHTML = '';
    let controlsHTML = '';
    if (canToggleAccess) {
      controlsHTML = `
        <button type="button" class="btn btn-sm ${m.suspended ? 'btn-primary' : 'btn-secondary'} mt-2" data-action="toggle-member-access" data-user-id="${m.userId}" data-suspended="${m.suspended ? '1' : '0'}">${m.suspended ? 'Enable access' : 'Disable access'}</button>`;
    }
    if (isOwner && !isSelf) {
      let roleOptions = '';

      // If they're the last owner, only show owner as an option.
      if (isLastOwner) {
        roleOptions = `
          <p class="text-secondary mb-2" style="font-size:12px">This is the only owner. Their role cannot be changed.</p>`;
      } else {
        roleOptions = `
          <div class="member-role-row">
            <label class="form-label" style="font-size:11px">Role</label>
            <select data-action="update-member-role" data-user-id="${m.userId}" data-is-owner="${m.role === 'owner' ? '1' : '0'}" class="role-select role-select--wide">
              <option value="owner" ${m.role === 'owner' ? 'selected' : ''}>Owner</option>
              <option value="admin" ${m.role === 'admin' ? 'selected' : ''}>Admin</option>
              <option value="member" ${m.role === 'member' ? 'selected' : ''}>Member</option>
            </select>
          </div>
          <button type="button" class="btn btn-sm btn-danger mt-2" data-action="remove-member" data-user-id="${m.userId}">Remove from household</button>`;
      }

      detailsHTML = `<div class="member-row-details">
        ${roleOptions}${controlsHTML}
      </div>`;
    } else if (isOwner && isSelf) {
      detailsHTML = `<div class="member-row-details">
        <p class="text-secondary" style="font-size:12px">You are the owner${ownerCount > 1 ? ' (one of ' + ownerCount + ')' : ''} of this household.</p>
      </div>`;
    } else if (isAdmin && canToggleAccess) {
      detailsHTML = `<div class="member-row-details">${controlsHTML}</div>`;
    }

    return `<details class="member-row" data-user-id="${m.userId}">
      <summary class="member-row-summary">
        <span class="avatar-circle-sm" style="background:${escapeHTML(m.avatarColor || '#19323C')}">${escapeHTML(initial)}</span>
        <span class="member-name">${escapeHTML(label)}</span>
        <span class="role-badge">${m.role}</span>
        ${m.suspended ? '<span class="role-badge member-disabled-badge">Disabled</span>' : ''}
        <span class="verify-badge${m.emailVerified ? ' verify-badge--ok' : ' verify-badge--no'}">${m.emailVerified ? 'Email \u2713' : 'Email \u2717'}</span>
        ${chevronSVG}
      </summary>
      ${detailsHTML}
    </details>`;
  }).join("");
  const inviteList = (invites || []).map(inv => {
    const invUrl = `${window.location.origin}/join?code=${encodeURIComponent(inv.code)}`;
    return `<li class="invite-item">
    <code class="invite-link-url">${escapeHTML(invUrl)}</code>
    <button type="button" class="btn btn-sm btn-secondary" data-action="copy-invite-link" data-code="${escapeHTML(inv.code)}">Copy</button>
    <span class="text-secondary">${inv.usedCount}/${inv.maxUses || 1} uses${inv.expiresAt ? ` · Expires ${escapeHTML(new Date(inv.expiresAt).toLocaleDateString())}` : ""}</span>
    <button type="button" class="btn btn-sm btn-danger" data-action="delete-invite" data-invite-id="${inv.id}">Revoke</button>
  </li>`;}).join("");

  const inviteLink = `${window.location.origin}/join?code=${encodeURIComponent(household.inviteCode)}`;

  const inviteSection = isOwner ? `
    <p class="text-secondary mb-1">Invite Link</p>
    <div class="invite-link-row">
      <code class="invite-link-url">${escapeHTML(inviteLink)}</code>
      <button type="button" class="btn btn-sm btn-secondary" data-action="copy-invite-link" data-code="${escapeHTML(household.inviteCode)}">Copy</button>
      <button type="button" class="btn btn-sm btn-secondary" data-action="toggle-invite-qr" aria-expanded="${showInviteQR}">${showInviteQR ? "Hide QR code" : "QR code"}</button>
    </div>
    ${showInviteQR ? renderInviteQR(inviteLink) : ""}
    <div class="mt-2">
      <button type="button" class="btn btn-sm btn-primary" data-action="create-invite">New single-use link</button>
      <p class="text-secondary">New links expire after 7 days. Removing a member or changing a role revokes existing links.</p>
    </div>
    ${invites && invites.length ? `<h4 class="mt-3">Active Invites</h4><ul class="invite-list">${inviteList}</ul><div class="auth-divider"></div>` : ''}` : '';

  return `<div class="card mt-3">
    <div class="hh-card-header">
      <span class="hh-initials-badge" aria-hidden="true">${escapeHTML(household.initials || '')}</span>
      <h3>${escapeHTML(household.name)}</h3>
      ${isOwner ? `<button type="button" class="btn btn-sm btn-secondary hh-edit-btn" data-action="toggle-edit-household">Edit</button>` : ''}
    </div>
    ${isOwner ? `<form id="edit-household-form" class="hh-edit-form hidden" data-action="update-household">
      <div class="form-group">
        <label class="form-label" for="edit-hh-name">Household Name</label>
        <input id="edit-hh-name" type="text" name="name" required value="${escapeHTML(household.name)}" data-autoinitials="edit-hh-initials">
      </div>
      <div class="form-group">
        <label class="form-label" for="edit-hh-initials">Initials <span class="form-label-hint">(shown in header)</span></label>
        <input id="edit-hh-initials" type="text" name="initials" maxlength="4" value="${escapeHTML(household.initials || '')}" autocomplete="off" class="hh-initials-input">
      </div>
      <div class="hh-edit-form-actions">
        <button type="submit" class="btn btn-sm btn-primary">Save</button>
        <button type="button" class="btn btn-sm btn-secondary" data-action="toggle-edit-household">Cancel</button>
      </div>
    </form>` : ''}
    ${inviteSection}
    <h4 class="mt-4">Members</h4>
    <div class="member-list">${memberList}</div>
    <div class="hh-leave-section">
      <p class="text-secondary">Leave this household. You can rejoin later with an invite link.</p>
      <button type="button" class="btn btn-sm btn-danger" data-action="leave-household">Leave Household</button>
    </div>
  </div>`;
}

export function generateInitials(name) {
  const words = (name || "").trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return "";
  if (words.length === 1) {
    return words[0].length <= 3 ? words[0].toUpperCase() : words[0].slice(0, 2).toUpperCase();
  }
  return words.map(w => w[0]).join("").toUpperCase();
}
