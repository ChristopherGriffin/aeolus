# 0130. The way online, a point at a time

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff. Tests should have maps of their steps, like the dataplane map of a VXLAN tunnel, each point with how long it took to achieve; later the times are trended; and every client that gets on is a test too, to monitor and plot. Written up by Claude
- Refines: 0118, 0121, 0123, 0127, 0128

## Context

- **A client test (0121, 0128) and a real client's own attempt (0118) are the same thing seen from two sides:** the steps of coming online, in order, each at a time. Both were shown as a row of chips, done or failed, and a table of steps under it.
- **How long each step took was there, and nobody could see it:** in the table, as a time after the start on each line. The one number said outright was the total.
- **A tunnel's path is drawn** (0059): where it starts, the tunnel, the VNI, the far end, each leg coloured by how the probers find it. Someone reads where it breaks at a glance.
- **Over the wire the two seconds a new device waits for its address were one number,** "address in 2.0 s". Whether the server is slow to offer or slow to confirm, the test knew and did not say apart.
- **What many tests, or many clients, come to at each step is what shows a site's habit:** that DHCP there takes a quarter of a second, and today takes four.

## Decision

- **The way online is a list of points in order,** each with how it went and how long it took, in ms:
  - **through the air:** found, authentication, association, the sign-in where the network has one, the keys, the address, the gateway, DNS, and the speed where it was measured;
  - **over the wire:** the offer of an address, the address given, the gateway, DNS, and the address given back;
  - **a client's own attempt:** authentication, association, the sign-in where there is one, the keys, the address, the gateway, DNS, its first connection.
- **A point is ok, bad, begun or none:** ok or bad as its last step that says went, and bad where the whole stopped there; begun where it was started and not seen through; none where nothing was seen of it.
  - A look that missed the network, and the next that found it, went well.
  - None is not a fault. A client that kept its address asks for none: its agent says it has one, and that point is ok, with no time.
  - A point left out where nothing was seen of it: the sign-in, the speed, the offer and the address given back. So a way is not always as long.
- **How long a point took is, the first of these that applies:**
  - **its answer's own time,** where the step says it: the address a test was given, a gateway's ping or ARP answer, a name looked up, a first connection;
  - **from its first step to its end,** where it began with a step that only asks: a client's discover to its acknowledgement, the scan radio's look to the network found;
  - **from the point before it to its end:** association after authentication, the keys after association. Never what a client asks for when it chooses, an address, its gateway, a name: how long after its keys it asked says nothing of the network.
- **Over the wire the address's time is DHCP's own, less the offer's.** Its step comes 300 ms after its acknowledgement did: the test looks for another that uses the address first (0128), and that is the test's time, not the server's.
- **A point that was done more than once is done at its first success** where it is a client's lookups or connections, which go on for as long as it stays. The others are done at their last step.
- **A gateway that answers ARP and no ping has no time.** The seconds its pings took say nothing of it.
- **A point that failed says how long it was at it:** an address asked for and not given for ten seconds failed after ten seconds.
- **A client's first point has no time.** Its steps are timed from its first, which is all its AP saw of it.
- **The manager works the way out, as a test or an attempt is read,** from the steps that were kept. So what was recorded before this has a way too, and one reading serves the page, the API and whatever trends them. An AP does not say its way, and one that does is not believed.
- **A step may name its point, `k`, where that is not its stage.** Over the wire, an agent from now on names the steps of the offer, and the address given back: steps of DHCP all. A test by an agent from before has its address as one point, with all of DHCP's time.
  - **A point the manager does not know is left out as the result is kept,** and the step is read as its stage. The result is not refused: an AP whose result is refused holds its later tests up.
- **A test with no steps left has what it measured:** an address, its gateway, a name, each ok with its time.
- **What many ways come to is the same list:** at each point how many passed it, how many stopped at it, and of the times of those that passed it the least, the middle and the most.
  - **A way stopped where it failed, and nowhere else.** A point that went badly in a way that went on, as a gateway that answers nothing before a name that is looked up all the same, is neither passed nor where it stopped.
  - **A thing tested (0123)** has it over its tests of the time shown.
  - **A network's joins (0127)** have it over its clients' attempts of the time shown, and never the tests' own client's. Where the APs the caller may view have more than 2000 between them, the latest 2000 of those are read: the most is counted of what the caller may see, and says nothing of the rest.
- **The page draws a way as it draws a tunnel's path:** a box a point, joined in order, green, red, amber where it was begun, grey where nothing was seen; in the box how long it took, and under it what was learned there: the channel, the signal, the address, the gateway. A test over the wire starts from the port and VLAN, or the tunnel, it was made from.
  - **Where:** a test opened, on an AP's page and on the Assurance tab; a client's attempt opened, in its journey; a thing tested, over its tests; a network's joins, over its attempts.
  - **What many come to** has the middle of the times in the box, and under it how many stopped there, or else the least and the most.
  - The table of steps stays under it.
- **In the API:** a client test's `detail.way`, wherever a test is listed; a connection's `way`; a check's `way`, and a network's `way` and `way_of`, in `GET /v1/assurance`.

## Consequences

- **Where the time goes is read off the path.** In the lab, over the wire: the offer after 2.0 s, the address a few ms after that, the gateway in 2 ms. The wait is the server's ping check before it offers (0128).
- **A test and a client can be set side by side:** the same points, timed the same way but for what only one of them can see. A test knows when it began to look; an AP sees a client first at its authentication.
- **The times are worked out at each reading, and kept nowhere.** To trend them over weeks they want keeping: a row a point, by network, AP and band, tests and clients alike. That is the next step, and this reading is what would fill it.
- **A network's way reads up to 2000 attempts whole at each look.** That is some tens of ms; on a site of thousands of clients it wants the kept rows above.
- **A point that failed by waiting says less than it waited,** where its agent notes no step at the end of the wait: a network looked for and never found says when its last look began. The test's own reason says the rest.
- **Not in this:** a step saying what the radio saw, frames sent, acknowledged and heard, so a test that hears nothing back says where the frames stopped.
