# Investigation method

Scale the effort to the question. These tiers are a ladder, not a checklist — name
the tier you are on in your first reply, and move up if the question turns out to
be bigger than it looked.

- **Quick** — one primary source settles it. Answer, cite, stop. No hypothesis
  work, one pass, under ten tool calls.
- **Standard** (default) — frame the question, carry two or more competing answers,
  gather evidence that tells them apart, conclude with a confidence and a
  falsification condition.
- **Deep** — for consequential, irreversible or contested questions. Split into
  independent sub-questions and research them in parallel, check the load-bearing
  assumptions, then argue against your own conclusion before writing it down.

## Before you search

- Write the question in one sentence, and say what decision it feeds. If the answer
  changes nothing, say so instead of researching anyway.
- Write your best guess now, before evidence. It is a target to attack, not a
  position to defend.
- Write at least two competing answers, including the boring one — nothing is
  wrong, it is a coincidence, the premise is false.
- Write what would prove you wrong. If no observation could, the question is not
  answerable yet; reframe it.
- Decide the stopping rule now — a source budget, a timebox, or a confidence
  threshold. A stopping rule chosen mid-investigation never fires.

## While you search

- Start broad and narrow afterwards. A long, over-specific first query finds only
  what you already believe.
- Prefer evidence that discriminates between your candidates. Evidence that fits
  every candidate is worth nothing, however much of it there is.
- Go to the primary source — the code, the log, the original paper, the raw metric.
  Trace a secondary claim back to what it actually rests on.
- Corroboration counts only across independent sources. Ten pages repeating one
  post are one source.
- Make independent tool calls in parallel rather than one after another.
- Delegate a sub-question to a subagent when it would flood this thread with
  material you do not need. Give it the exact question, the format you want back,
  which sources to prefer, and what is out of scope. Ask for conclusions, not
  transcripts.
- Record what you looked for and did not find, and whether that absence means
  anything — you looked where it would have shown up, or you did not really look.
- Stop when new sources stop changing the answer, or when your stopping rule fires.

## While you write

- Answer first, then the reasons, then the evidence — not a chronology of what you
  did.
- Mark every claim as fact, inference or assumption. A fact carries its source: a
  URL, a `file:line`, the command you ran. An inference says what it is derived
  from. An assumption says what breaks if it is false.
- Never invent a citation, a version, a number or a quote. If you cannot support a
  claim, mark it unverified or drop it.
- Give each conclusion a confidence — high, medium, low — and say what it rests on.
  "I do not know" is a useful answer; a confident guess is not.
- Apply the "so what" test to every finding. If it would not change anyone's
  decision, move it to the log.
- Write conclusions as prose. A bullet list of facts reads as a summary, not as an
  analysis.
- Keep `README.md` current as you go. It is the note file that survives a context
  reset, so read it first when you resume.

## Before you call it done

- Argue the strongest case for the runner-up answer, in writing.
- Name the assumptions the conclusion rests on, and which one breaking would break
  it.
- Say plainly what you could not check and what is still unresolved.
- If the evidence contradicts what was hoped for, lead with that. Agreement you do
  not hold is worse than no answer.
