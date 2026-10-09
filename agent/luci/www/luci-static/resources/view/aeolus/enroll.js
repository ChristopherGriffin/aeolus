'use strict';
'require view';
'require fs';
'require ui';
'require poll';
'require dom';

// LuCI's Aeolus page (0083): join this AP to a manager named by hand, for
// an AP that found none by itself. It runs aeolus-enroll, which fetches the
// manager's certificate, shows its fingerprint for a person to check, and
// joins the AP once they have: the agent pins that certificate (0033).

const ENROLL = '/usr/libexec/aeolus-enroll';

function call(args) {
	return fs.exec(ENROLL, args).then(res => {
		if (res.code != 0)
			throw new Error((res.stderr || res.stdout || '').trim() || _('aeolus-enroll failed'));
		return JSON.parse(res.stdout);
	});
}

// spaced writes a SHA-256 as browsers do: AB:CD:...
function spaced(hex) {
	return (hex || '').toUpperCase().replace(/(..)(?!$)/g, '$1:');
}

function bare(fp) {
	return (fp || '').replace(/[\s:]/g, '').toLowerCase();
}

function row(title, field, description) {
	return E('div', { 'class': 'cbi-value' }, [
		E('label', { 'class': 'cbi-value-title' }, title),
		E('div', { 'class': 'cbi-value-field' }, [
			field,
			description ? E('div', { 'class': 'cbi-value-description' }, description) : ''
		])
	]);
}

function fail(err) {
	ui.addNotification(null, E('p', err.message), 'danger');
}

return view.extend({
	load() {
		return Promise.all([call(['status']), call(['ports']).catch(() => ({ ports: [] }))]);
	},

	renderStatus(st) {
		const lines = [
			[_('Manager'), st.url || _('none')],
			[_('Uplink'), st.uplink || _('none')],
			[_('Certificate SHA-256'), st.fingerprint ? spaced(st.fingerprint) : _('none')],
			[_('Agent'), st.running == 'yes' ? _('running') : _('not running')],
			[_('Enrolled'), st.enrolled == 'yes' ? _('yes') : _('not yet')]
		];
		return E('div', {}, [
			E('table', { 'class': 'table' }, lines.map(([k, v]) => E('tr', { 'class': 'tr' }, [
				E('td', { 'class': 'td left', 'width': '33%' }, k),
				E('td', { 'class': 'td left', 'style': 'word-break:break-all' }, v)
			]))),
			st.job != 'none' ? E('div', {}, [
				E('h4', {}, st.job == 'running' ? _('Joining…') : st.job == 'done' ? _('Joined') : _('Joining failed')),
				E('pre', { 'style': 'white-space:pre-wrap' }, st.log || '')
			]) : ''
		]);
	},

	render([status, ports]) {
		const statusBox = E('div', {}, this.renderStatus(status));
		const choices = (ports.ports || []).slice();
		const current = status.uplink || status.guess || '';
		if (current && !choices.includes(current))
			choices.unshift(current);

		const manager = E('input', {
			'class': 'cbi-input-text', 'type': 'text', 'spellcheck': 'false',
			'placeholder': 'aeolus.example.com, 192.168.20.60 or [fd00::60]:8443',
			'value': ''
		});
		const uplink = E('select', { 'class': 'cbi-input-select' }, [
			E('option', { 'value': '' }, _('the one the default route leaves by')),
			...choices.map(p => E('option', p == current ? { 'value': p, 'selected': '' } : { 'value': p }, p))
		]);
		const shown = E('code', { 'style': 'word-break:break-all' }, '');
		const expected = E('input', {
			'class': 'cbi-input-text', 'type': 'text', 'spellcheck': 'false',
			'placeholder': _('optional: paste the fingerprint to compare')
		});
		const verdict = E('span', {}, '');
		const confirm = E('input', { 'type': 'checkbox' });
		const enroll = E('button', { 'class': 'btn cbi-button cbi-button-apply', 'disabled': '' }, _('Join this manager'));
		const certBox = E('div', { 'style': 'display:none' }, [
			row(_('Its certificate'), shown,
				_('The SHA-256 of the certificate the manager sent. Check it against the manager’s own: aeolus fingerprint, on the manager, or the fingerprint a browser shows for it.')),
			row(_('Compare'), E('div', {}, [expected, ' ', verdict])),
			row(_('Trust it'), E('label', {}, [confirm, ' ', _('This is my manager’s fingerprint')]))
		]);
		let fetched = null;

		const update = () => {
			const want = bare(expected.value);
			if (!want)
				verdict.textContent = '';
			else if (fetched && want == fetched.fingerprint)
				verdict.textContent = '✔ ' + _('matches');
			else
				verdict.textContent = '✘ ' + _('does not match');
			const ok = fetched && (confirm.checked || want == fetched.fingerprint) && !(want && want != fetched.fingerprint);
			enroll.disabled = !ok;
		};
		expected.addEventListener('input', update);
		confirm.addEventListener('change', update);
		manager.addEventListener('input', () => {
			fetched = null;
			certBox.style.display = 'none';
			confirm.checked = false;
			update();
		});

		const fetchCert = E('button', {
			'class': 'btn cbi-button cbi-button-action',
			'click': ui.createHandlerFn(this, () => {
				const m = manager.value.trim();
				if (!m)
					return fail(new Error(_('Give the manager’s name or address.')));
				return call(['cert', m]).then(res => {
					fetched = res;
					shown.textContent = spaced(res.fingerprint);
					certBox.style.display = '';
					confirm.checked = false;
					update();
				}).catch(fail);
			})
		}, _('Fetch its certificate'));

		const refresh = () => call(['status']).then(st => {
			dom.content(statusBox, this.renderStatus(st));
			if (st.job != 'running')
				poll.remove(refresh);
		}).catch(() => {});

		enroll.addEventListener('click', ui.createHandlerFn(this, () => {
			if (!fetched)
				return;
			const args = ['start', fetched.url, fetched.fingerprint];
			if (uplink.value)
				args.push(uplink.value);
			enroll.disabled = true;
			return call(args).then(() => {
				poll.add(refresh, 2);
				return refresh();
			}).catch(err => {
				fail(err);
				update();
			});
		}));

		if (status.job == 'running')
			poll.add(refresh, 2);

		return E('div', { 'class': 'cbi-map' }, [
			E('h2', {}, _('Aeolus')),
			E('div', { 'class': 'cbi-map-descr' },
				_('Joins this AP to an Aeolus manager named here, for an AP that found none by itself. The AP then waits in the manager’s Landing Zone for a person to adopt it.')),
			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('This AP')),
				statusBox
			]),
			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Join a manager')),
				row(_('Manager'), manager,
					_('Its DNS name or IP address, reached on port 8443 unless another is given. A name is added to dnsmasq’s rebind_domain, so the AP can resolve it to a private address.')),
				row(_('Uplink'), uplink, _('The port carrying the VLANs.')),
				row('', fetchCert),
				certBox,
				row('', enroll)
			])
		]);
	},

	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
