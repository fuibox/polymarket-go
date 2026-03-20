# BASE_BACKEND_RULES.md

Backend Agent Base Rules (Project-agnostic)

These rules are intended to be reusable across backend repositories, especially for financial / trading / concurrency-sensitive systems.

If project-specific rules conflict with these, **stop and escalate**.

---

## 0) Priorities

Always prioritize:
1. correctness
2. idempotency
3. minimal code changes
4. short transactions
5. safe concurrency

Never sacrifice correctness for refactoring elegance.

---

## 1) Financial Correctness

### 1.1 Use decimal, not float
Never use float/double for money, price, quantity.
Use a decimal library (e.g., `shopspring/decimal` in Go).

### 1.2 No duplicate balance mutation
The system must guarantee:
- no duplicate balance mutation
- no duplicate trade creation
- no duplicate rollback
- no duplicate withdraw processing
- no duplicate settlement
- no negative remaining quantities

Financial correctness is the highest priority.

---

## 2) Idempotency & Concurrency

### 2.1 Idempotency is mandatory
Assume operations may execute more than once due to:
- task retries
- worker duplication
- event replay from external systems
- partial transaction failure

Duplicate execution must not create duplicate side effects.

### 2.2 Guarded update pattern (optimistic concurrency)
Prefer conditional updates:

```sql
UPDATE table
SET status = 'done'
WHERE id = ? AND status = 'pending';
```

After update:
- rows affected == 1 → success
- rows affected == 0 → already completed or another worker succeeded

When rows affected == 0:
- skip the operation
- emit a **warning** log with enough context (ids, current state, trigger)

Avoid unconditional state updates.

### 2.3 Assume concurrent execution
Multiple workers may process the same record simultaneously.
Never assume exactly-once semantics.

### 2.4 Prefer optimistic over pessimistic locking
Use guarded updates instead of `SELECT FOR UPDATE` where possible.
Reserve pessimistic locks for cases that cannot be made safe otherwise.

---

## 3) Transaction Safety

### 3.1 Keep transactions short
Transactions protect only the minimal critical section.

### 3.2 Never do external calls inside transactions
Inside a DB transaction, **forbid**:
- HTTP requests
- RPC / blockchain calls
- external service calls
- long loops
- slow IO

If the external call fails, the transaction must not be left in an ambiguous state.

### 3.3 Lock timeout handling
If you see "Lock wait timeout exceeded", investigate:
1. oversized transactions
2. unnecessary row locks
3. inconsistent update order
4. concurrent workers touching the same rows
5. missing guarded completion logic

---

## 4) Code Modification Discipline (Agent Hygiene)

### 4.1 Read before edit
Always read the relevant file(s) before modifying.

### 4.2 Avoid scanning the entire repository
Use known entry points (README, project map, router/main).
Targeted reads reduce noise and mistakes.

### 4.3 Minimum code changes
Change as little as possible to solve the problem.
Do not refactor unrelated code.

### 4.4 Stop at three files
If a fix requires modifying more than **three files**:
- stop
- re-check the root cause
- explain why broader changes are necessary before continuing

Large patches often indicate the fix is happening at the wrong level.

### 4.5 No speculative improvements
Do not introduce abstractions, renames, or cleanups not required for the issue.

---

## 5) Architecture Boundaries

### 5.1 No business logic in handlers
Handlers validate input and call the service layer.
State transitions and DB writes belong in services.

### 5.2 Validate at system boundaries only
Validate:
- user input
- external API responses / RPC results

Trust internal function calls, ORM results, and framework guarantees.
Avoid adding defensive checks deep inside internal code paths.

### 5.3 Respect layer boundaries
Dependencies flow: handler → service → model/db.
Do not import upward.
Services must not call handler-level logic.
Do not bypass the service layer.

---

## 6) Pre-modification Workflow (recommended)

Before modifying code:
1. identify the exact module involved
2. read the minimal relevant files
3. understand state transitions
4. consider retry and concurrency behavior
5. implement the smallest safe fix

Avoid scanning the entire repository unless necessary.

---

## 7) Output / Reporting (for AI agents)

When completing a task, summarize briefly:
- root cause
- files changed
- why the fix is safe
- concurrency/transaction implications
- remaining risks

### 7.1 Pre-submission review checklist (financial/concurrency)
Before finalizing a change, verify:
- can this operation run twice safely?
- could another worker run simultaneously?
- are state transitions guarded?
- could balance mutate twice?
- could a trade be created twice?
- could quantity become negative?
- is transaction scope minimal?
