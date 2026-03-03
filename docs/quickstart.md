# Quickstart

This guide shows a full baseline -> attempt -> compare -> restore loop.

## 1. Initialize tracking

```bash
converge init
```

This creates `.converge/` and `.convergeignore` in your repository.

## 2. Create baseline cell

```bash
converge snap -m "baseline" --eval=false
```

Expected output includes a new cell id like `c_000001`.

## 3. Make code changes and snapshot again

```bash
converge snap -m "attempt 2" --eval=false
```

Expected output includes `c_000002`.

## 4. Inspect history and differences

```bash
converge log
converge status
converge diff c_000001 c_000002
```

## 5. Optional semantic compare

```bash
export OPENAI_API_KEY=sk-...
converge compare c_000001 c_000002
```

If `OPENAI_API_KEY` is missing, Converge returns a clear actionable error.

## 6. Branch experiments

```bash
converge fork feature-a --switch
# edit files
converge snap -m "feature-a attempt" --eval=false
converge switch main
converge branches
```

## 7. Restore a previous state safely

```bash
converge restore c_000001
```

Restore creates a safety cell first, then materializes tracked files from target cell.

## 8. Open dashboard

```bash
converge ui --addr 127.0.0.1:7777
```

Then open `http://127.0.0.1:7777` in a browser.
