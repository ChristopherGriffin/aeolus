// Writing UCI packages as text (0039, 0040).
//
// A package is what uci's get_all() returns: an object of sections by name,
// each with ".type", ".name", ".anonymous" and ".index", and its options as
// strings or, for lists, arrays of strings. The text is `uci export` form;
// it is what the agent sends for its render check and, without the package
// line, what it writes to /etc/config, so what is checked is what runs.

'use strict';

// quote writes a value in single quotes, a quote inside as '\''.
function quote(v) {
	return "'" + replace('' + v, "'", "'\\''") + "'";
}

// sections returns a package's sections in their order.
function sections(pkg) {
	let list = values(pkg ?? {});
	sort(list, (a, b) => (a['.index'] ?? 0) - (b['.index'] ?? 0));
	return list;
}

function body(pkg) {
	let out = '';
	for (let s in sections(pkg)) {
		out += '\nconfig ' + s['.type'] + (s['.anonymous'] ? '' : ' ' + quote(s['.name'])) + '\n';
		for (let k in s) {
			if (substr(k, 0, 1) == '.')
				continue;
			if (type(s[k]) == 'array') {
				for (let v in s[k])
					out += '\tlist ' + k + ' ' + quote(v) + '\n';
			}
			else {
				out += '\toption ' + k + ' ' + quote(s[k]) + '\n';
			}
		}
	}
	return out;
}

// text writes one package as `uci export` does.
function text(name, pkg) {
	return 'package ' + name + '\n' + body(pkg);
}

// file writes one package as a file in /etc/config.
function file(pkg) {
	return substr(body(pkg), 1);
}

// Exported in one statement: this ucode version cannot parse a comment
// that follows an exported function declaration.
export { quote, sections, text, file };
