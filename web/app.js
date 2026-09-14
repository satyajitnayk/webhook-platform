let apiKey = sessionStorage.getItem('webhook_api_key') || '';

const apiKeyInput = document.getElementById('apiKey');

console.log('apiKey', apiKeyInput);

if (apiKey) {
  apiKeyInput.value = apiKey;
}

/* -------------------------
   Helpers
------------------------- */

function showError(message) {
  const element = document.getElementById('error');

  element.textContent = message;
  element.classList.remove('hidden');

  document.getElementById('success').classList.add('hidden');
}

function showSuccess(message) {
  const element = document.getElementById('success');

  element.textContent = message;
  element.classList.remove('hidden');

  document.getElementById('error').classList.add('hidden');
}

function clearMessages() {
  document.getElementById('error').classList.add('hidden');

  document.getElementById('success').classList.add('hidden');
}

function formatDate(value) {
  if (!value) {
    return '-';
  }

  return new Date(value).toLocaleString();
}

function escapeHtml(value) {
  const div = document.createElement('div');

  div.textContent = value ?? '';

  return div.innerHTML;
}

/* -------------------------
   API
------------------------- */

async function apiFetch(path, options = {}) {
  if (!apiKey) {
    throw new Error('Enter the API key first.');
  }

  const headers = {
    'X-API-Key': apiKey,
    ...(options.headers || {}),
  };

  const response = await fetch(path, {
    ...options,
    headers,
  });

  if (!response.ok) {
    let message = `Request failed (${response.status})`;

    try {
      const body = await response.json();

      if (body.error) {
        message = body.error;
      }
    } catch {
      // Response wasn't JSON.
    }

    throw new Error(message);
  }

  return response.json();
}

/* -------------------------
   Webhooks
------------------------- */

async function loadWebhooks() {
  const webhooks = await apiFetch('/webhooks');

  document.getElementById('webhookCount').textContent = webhooks.length;

  const table = document.getElementById('webhooksTable');

  table.innerHTML = '';

  if (webhooks.length === 0) {
    table.innerHTML = `
            <tr>
                <td colspan="3">
                    No webhooks registered.
                </td>
            </tr>
        `;

    return;
  }

  for (const webhook of webhooks) {
    const row = document.createElement('tr');

    row.innerHTML = `
            <td>
                ${escapeHtml(webhook.url)}
            </td>

            <td>
                ${formatDate(webhook.created_at)}
            </td>

            <td>
                ${escapeHtml(webhook.id)}
            </td>
        `;

    row.addEventListener('click', () => {
      loadWebhookDetails(webhook.id);
    });

    table.appendChild(row);
  }

  await loadWebhookOptions();
}

/* -------------------------
   Webhook Details
------------------------- */

async function loadWebhookDetails(id) {
  try {
    clearMessages();

    const webhook = await apiFetch(`/webhooks/${encodeURIComponent(id)}`);

    document.getElementById('detailWebhookId').textContent = webhook.id;

    document.getElementById('detailWebhookUrl').textContent = webhook.url;

    document.getElementById('detailWebhookCreated').textContent = formatDate(
      webhook.created_at,
    );

    document.getElementById('detailDeliveryCount').textContent =
      webhook.delivery_count;

    const events = document.getElementById('detailEvents');

    events.innerHTML = '';

    if (!webhook.events || webhook.events.length === 0) {
      events.textContent = 'No subscribed events.';
    } else {
      for (const event of webhook.events) {
        const tag = document.createElement('span');

        tag.className = 'event-tag';

        tag.textContent = event;

        events.appendChild(tag);
      }
    }

    document.getElementById('webhookDetails').classList.remove('hidden');

    document.getElementById('webhookDetails').scrollIntoView({
      behavior: 'smooth',
    });
  } catch (error) {
    showError(error.message);
  }
}

document
  .getElementById('eventForm')
  .addEventListener('submit', async (event) => {
    event.preventDefault();

    clearMessages();

    const webhookId = document.getElementById('eventWebhook').value;

    const eventType = document.getElementById('eventType').value.trim();

    const payloadText = document.getElementById('eventPayload').value.trim();

    if (!webhookId) {
      showError('Select a webhook.');
      return;
    }

    let payload;

    try {
      payload = JSON.parse(payloadText);
    } catch {
      showError('Payload must be valid JSON.');
      return;
    }

    try {
      const result = await apiFetch('/events', {
        method: 'POST',

        headers: {
          'Content-Type': 'application/json',
        },

        body: JSON.stringify({
          type: eventType,
          payload: payload,
        }),
      });

      showSuccess(`Event created. ${result.deliveries} delivery created.`);

      await loadDeliveries();
    } catch (error) {
      showError(error.message);
    }
  });

/* -------------------------
   Create Webhook
------------------------- */

document
  .getElementById('webhookForm')
  .addEventListener('submit', async (event) => {
    event.preventDefault();

    clearMessages();

    const url = document.getElementById('webhookUrl').value.trim();

    const eventTypes = document
      .getElementById('eventTypes')
      .value.split(',')
      .map((value) => value.trim())
      .filter(Boolean);

    if (eventTypes.length === 0) {
      showError('Enter at least one event type.');
      return;
    }

    try {
      const result = await apiFetch('/webhooks', {
        method: 'POST',

        headers: {
          'Content-Type': 'application/json',
        },

        body: JSON.stringify({
          url: url,
          events: eventTypes,
        }),
      });

      document.getElementById('webhookForm').reset();

      showSuccess(`Webhook created. Secret: ${result.secret}`);

      await loadWebhooks();
    } catch (error) {
      showError(error.message);
    }
  });

/* -------------------------
   Deliveries
------------------------- */

async function loadDeliveries() {
  const deliveries = await apiFetch('/deliveries');

  document.getElementById('deliveryCount').textContent = deliveries.length;

  let success = 0;
  let failed = 0;
  let pending = 0;

  for (const delivery of deliveries) {
    switch (delivery.status) {
      case 'success':
        success++;
        break;

      case 'failed':
        failed++;
        break;

      case 'pending':
      case 'processing':
        pending++;
        break;
    }
  }

  document.getElementById('successCount').textContent = success;

  document.getElementById('failedCount').textContent = failed;

  document.getElementById('pendingCount').textContent = pending;

  const table = document.getElementById('deliveriesTable');

  table.innerHTML = '';

  if (deliveries.length === 0) {
    table.innerHTML = `
            <tr>
                <td colspan="5">
                    No deliveries yet.
                </td>
            </tr>
        `;

    return;
  }

  for (const delivery of deliveries) {
    const row = document.createElement('tr');

    row.innerHTML = `
            <td>
                ${escapeHtml(delivery.id)}
            </td>

            <td>
                ${escapeHtml(delivery.webhook_id)}
            </td>

            <td>
                <span class="status status-${escapeHtml(delivery.status)}">
                    ${escapeHtml(delivery.status)}
                </span>
            </td>

            <td>
                ${delivery.attempts}
            </td>

            <td>
                ${formatDate(delivery.created_at)}
            </td>
        `;

    row.addEventListener('click', () => {
      loadDeliveryDetails(delivery.id);
    });

    table.appendChild(row);
  }
}

/* -------------------------
   Delivery Details
------------------------- */

async function loadDeliveryDetails(id) {
  console.log('delivery id:', id);
  try {
    clearMessages();

    const delivery = await apiFetch(`/deliveries/${encodeURIComponent(id)}`);

    document.getElementById('detailDeliveryId').textContent = delivery.id;

    document.getElementById('deliveryStatus').innerHTML = `
                <span class="status status-${escapeHtml(delivery.status)}">
                    ${escapeHtml(delivery.status)}
                </span>
            `;

    document.getElementById('deliveryAttempts').textContent = delivery.attempts;

    document.getElementById('deliveryCreated').textContent = formatDate(
      delivery.created_at,
    );

    document.getElementById('deliveryDetails').classList.remove('hidden');

    document.getElementById('deliveryDetails').scrollIntoView({
      behavior: 'smooth',
    });
  } catch (error) {
    showError(error.message);
  }
}

/* -------------------------
   API Key
------------------------- */

document.getElementById('saveApiKey').addEventListener('click', async () => {
  const value = apiKeyInput.value.trim();

  if (!value) {
    showError('Enter the API key.');

    return;
  }

  apiKey = value;

  sessionStorage.setItem('webhook_api_key', apiKey);

  clearMessages();

  try {
    await refreshDashboard();

    showSuccess('Connected.');
  } catch (error) {
    sessionStorage.removeItem('webhook_api_key');

    apiKey = '';

    showError(error.message);
  }
});

/* -------------------------
   Refresh
------------------------- */

async function refreshDashboard() {
  clearMessages();

  await Promise.all([loadWebhooks(), loadDeliveries()]);
}

async function loadWebhookOptions() {
  const webhooks = await apiFetch('/webhooks');

  const select = document.getElementById('eventWebhook');

  select.innerHTML = `
        <option value="">
            Select a webhook
        </option>
    `;

  for (const webhook of webhooks) {
    const option = document.createElement('option');

    option.value = webhook.id;

    option.textContent = `${webhook.url} (${webhook.id})`;

    select.appendChild(option);
  }
}

document
  .getElementById('refreshWebhooks')
  .addEventListener('click', async () => {
    try {
      await loadWebhooks();
    } catch (error) {
      showError(error.message);
    }
  });

document
  .getElementById('refreshDeliveries')
  .addEventListener('click', async () => {
    try {
      await loadDeliveries();
    } catch (error) {
      showError(error.message);
    }
  });

/* -------------------------
   Close details
------------------------- */

document.getElementById('closeDetails').addEventListener('click', () => {
  document.getElementById('webhookDetails').classList.add('hidden');
});

document
  .getElementById('closeDeliveryDetails')
  .addEventListener('click', () => {
    document.getElementById('deliveryDetails').classList.add('hidden');
  });

/* -------------------------
   Initial load
------------------------- */

if (apiKey) {
  refreshDashboard().catch((error) => {
    showError(error.message);
  });
}
