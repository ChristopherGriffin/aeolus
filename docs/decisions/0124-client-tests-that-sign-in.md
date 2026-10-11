# 0124. Client tests that sign in

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff, among what should follow the client test (0121): "802.1X client tests". Written up by Claude
- Refines: 0027, 0098, 0121, 0123

## Context

- **A client test joins a network with what the AP has of it** (0121): the network's own passphrase, which the AP holds as it serves the network.
- **A network that signs each client in by 802.1X has no passphrase** (0098). A client signs in as a user, with a password or a certificate, against a RADIUS server the AP only passes it on to. The AP has no user of its own, so the test could not join such a network, and they were left out of the tests on a schedule too (0123).
- **These are the networks most in need of it.** More can fail on the way online: the server, its secret, the user's VLAN, the server's patience.
- **Secrets are sealed in the manager** (0027): written, never read back.

## Decision

- **A network may have a test user:** `radius.test.user`, `radius.test.password` and `radius.test.method`, with its other RADIUS settings.
  - **The user is one made at the RADIUS server for this,** with no more access than a test needs. Aeolus makes none: it is told one.
  - **The method** is `peap`, PEAP with MSCHAPv2, unless set; or `ttls`, TTLS with PAP.
  - **The password is sealed,** as a passphrase is.
- **A network that signs clients in can be tested once it has a user and a password.** Without, a test of it is refused, and says what is missing; the schedule (0123) leaves it out.
- **The password goes to the AP with the test, and nowhere else.**
  - **It is no part of an AP's config.** The manager leaves it out of what an AP is sent, and a change of it makes no AP's config new. The user's name and the method are in the config, as the rest of the network is.
  - **It is not kept with the test.** The action says who signs in and how. The manager opens the password and adds it only as it hands the test to the AP that makes it, read from the network as it stands then.
  - **Nobody reads it back:** not the person who asked, nor anyone who lists the AP's actions or reads what the test found.
  - **On the AP it is in memory,** in files only root reads: what was asked, removed once read, and wpa_supplicant's configuration, removed when the test ends.
- **The test signs in as a client would,** and the sign-in is a stage of its own, between association and the keys: started, the method the server chose, and signed in or refused.
  - WPA2-Enterprise, WPA3-Enterprise, and both at once, as the network is.
- **The RADIUS server's certificate is not checked.** The test asks whether a client with the right name and password is let on, not whom that client should trust.

## Consequences

- **An 802.1X network is tried end to end,** on a schedule or by hand: the AP's path to the RADIUS server, the server's answer, the keys, the VLAN the user lands in, DHCP there, its gateway and DNS.
- **The test user is a real account,** and its password is on each AP for the seconds a test takes. It should open nothing a guest could not reach. One the server puts in a VLAN of its own shows that VLAN's service, not the one staff get.
- **A test that is refused says the sign-in was refused, and no more.** Why is the RADIUS server's to say, in its own log.
- **A wrong certificate at the server goes unseen** by the test, though a careful client would refuse it.
- **A network of certificates alone (EAP-TLS) cannot be tested:** the test has a password, not a certificate.
- **An AP on an older agent** is handed a test with a user it does not know of, and says it cannot join a network that signs clients in.

## Tried

- **On the C-360, 2026-10-11, against the lab's FreeRADIUS,** as a user made for it and removed after, on the office AP's WPA2/WPA3-Enterprise network, 2.4 GHz:
  - **With its password:** associated at 0.34 s, the sign-in started at 0.55 s, PEAP chosen, signed in at 0.61 s, the keys at 0.75 s, an address at 1.0 s, its gateway and a name after. Passed.
  - **With a wrong password:** "failed at signin: the sign-in was refused", 1.4 s in.
  - **What was asked, password and all, was gone from the AP** once the test had read it.
- **In the manager's tests:** refused with no user, and with a name and no password; the password in the AP's hand-over and nowhere else: not in the action, the list of actions, the AP's config, nor the record after.

## Not done here

- **EAP-TLS,** with a certificate for the test.
- **Checking the server's certificate,** against a CA the network names.
- **Networks that give each device a passphrase of its own** (0115): the server would need the scan radio's MAC.
- **A different user for each AP,** where a server allows one sign-in at a time.
