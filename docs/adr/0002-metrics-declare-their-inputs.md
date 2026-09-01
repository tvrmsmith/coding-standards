# Each metric declares the inputs it needs, and the gate demands an input only when a selected metric asked for it

`metric-gate` hosts more than one metric. CRAP declares that it needs a coverage report; a metric computed from the extractor output alone declares nothing. The gate resolves which metrics the run selected, takes the union of their declared inputs, and only then goes looking. A run selecting only metrics that need no coverage passes in a repo where nobody ran the tests, because nothing asked for coverage and nothing was silently skipped.

A run that selects CRAP and finds no usable coverage report fails, and the message names the metric that is stuck rather than the file that is absent: "CRAP requires a coverage report, none found".

## Considered options

**The gate always requires a coverage report.** One rule, no declaration mechanism, no union step, and correct today because CRAP is the only metric. Rejected because the rule is written against the wrong subject. The gate does not consume coverage, CRAP does, and the day a second metric arrives the rule has to be unpicked rather than extended. The cost of getting it right now is a list of input names per metric, which is a handful of lines.

**Each metric fetches its own inputs.** Push the whole concern into the metric, so CRAP discovers, parses, and staleness-checks the coverage report itself. Rejected because two metrics needing the same report would discover and parse it twice, and would drift on the staleness rule. Declaration keeps the policy in one place and lets the gate parse each report once.

## Consequences

Missing, stale, and unparseable coverage are properties of the coverage **input**, and the rules for them belong to that input's contract rather than to the gate as a whole. That is what makes the answers recorded on [issue 6](https://github.com/tvrmsmith/coding-standards/issues/6) survive the arrival of a second metric untouched.

The map's open question of whether a second metric ever joins the mode can no longer invalidate the input handling. It changes which inputs a given run demands, not what happens when one of them is rotten.

A metric that declares an input it does not read costs a false failure, and one that reads an input it did not declare costs a nil dereference. The declaration is the only thing tying the two together, so it has to be checked against reality by test rather than by discipline.
