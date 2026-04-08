# Kairos Dashboard Redesign — visionOS Glassmorphism

**Date:** 2026-03-23
**Status:** Approved
**Mockup reference:** `.superpowers/brainstorm/26814-1774297817/full-mockup-v6.html`

## Overview

Complete visual redesign of the Kairos dashboard from the current retro-futuristic amber/cyan/violet theme to a clean Apple visionOS-inspired glassmorphism design with Frost (monochromatic + blue) palette.

## Design System

### Colors

| Token | Value | Usage |
|-------|-------|-------|
| `bg` | `#06060e` | Page background |
| `accent` | `#3b82f6` | Primary accent (buttons, active states, links) |
| `accent-light` | `#60a5fa` | Status indicators, glow effects |
| `accent-gradient` | `#3b82f6 → #818cf8` | Send button, logo, progress bars |
| `cyan` | `#22d3ee` | Online status dot |
| `error` | `rgba(248,113,113,0.7)` | Failed counts |
| `text-primary` | `rgba(255,255,255,0.9)` | Headings, user messages |
| `text-secondary` | `rgba(255,255,255,0.65)` | Body text, panel titles |
| `text-tertiary` | `rgba(255,255,255,0.3-0.45)` | Meta, labels, placeholders |
| `text-muted` | `rgba(255,255,255,0.18-0.25)` | Timestamps, subtle labels |

### Glass Layers

Two glass tiers — the key is low blur for real transparency:

| Tier | Background | Blur | Border | Border-top | Usage |
|------|-----------|------|--------|------------|-------|
| `glass` | `rgba(255,255,255,0.04)` | `blur(12px) saturate(140%)` | `rgba(255,255,255,0.1)` | `rgba(255,255,255,0.18)` | Sidebar, context panel, cards |
| `glass-elevated` | `rgba(255,255,255,0.06)` | `blur(16px) saturate(150%)` | `rgba(255,255,255,0.13)` | `rgba(255,255,255,0.22)` | Main chat panel, primary content |

Both tiers include:
- `box-shadow: 0 4-8px 20-30px rgba(0,0,0,0.2-0.25)`
- `inset 0 1px 0 rgba(255,255,255,0.06-0.08)` (top inner highlight)
- `border-radius: 18px`

### Typography

- **Font:** Inter (300, 400, 500, 600, 700)
- **Nav title:** 13px / 600 / -0.3px tracking
- **Nav links:** 12px / 500
- **Panel title:** 13px / 500
- **Body text:** 12-13px / 400
- **Labels:** 10px / uppercase / 0.8-2px letter-spacing
- **Stat values:** 24-30px / 300 / -1.5px tracking
- **Code:** SF Mono / JetBrains Mono

### Spacing & Radius

- Panel padding: 20px
- Panel gap: 12px
- Border-radius: 18px (panels), 14-16px (inputs, buttons), 10-12px (tabs, items)
- Container max-width: 1440px
- Nav top offset: 12px

## Layout

### Navigation — Floating Pill

Centered fixed pill at top of viewport:
```
[Logo K] Kairos | Chat  System  Memories  RAG | ● Online
```
- Glass background with same blur treatment
- Brand section separated by vertical border
- Status indicator on right, separated by vertical border
- Active tab: `rgba(255,255,255,0.1)` background, white text

### Background

- Solid dark: `#06060e`
- No decorative elements currently — reserved for future iteration
- Clean background lets glass borders and highlights define the panels

### Pages

4 views (down from 5 — Conversations merged into Chat):

1. **Chat** (default route `/`)
2. **System** (`/system`)
3. **Memories** (`/memories`)
4. **RAG** (`/rag`)

## Page Designs

### 1. Chat — Hybrid Split

Three-column layout filling viewport height:

```
┌──────────┬────────────────────────┬──────────┐
│ Sidebar  │     Main Chat          │ Context  │
│ 230px    │     flex: 1            │ 280px    │
│ glass    │     glass-elevated     │ glass    │
│          │                        │          │
│ Search   │  [messages]            │ Tabs:    │
│ Conv 1 ● │                        │ Memories │
│ Conv 2   │                        │ RAG      │
│ Conv 3   │                        │ Tools    │
│ Conv 4   │  ┌──────────────┐ ┌──┐ │          │
│          │  │ Input        │ │↑ │ │ [items]  │
│ [+New]   │  └──────────────┘ └──┘ │          │
│          │                        │ Model    │
│          │                        │ badge    │
└──────────┴────────────────────────┴──────────┘
```

**Sidebar:** Search input, conversation list (active = `rgba(255,255,255,0.08)`), "+ New Chat" button at bottom.

**Main chat:**
- Messages: user (blue bg, right-aligned, rounded with bottom-right small radius), assistant (white/5% bg, left-aligned, bottom-left small), tool calls (monospace, blue-tinted)
- Input: glass-styled input + gradient send button
- Model badge in header (e.g. "llama3.1 · 8k")

**Context panel (collapsible):**
- Tab bar: Memories / RAG / Tools
- List of context items relevant to current conversation
- Mini model status card at bottom
- Collapse button (‹) in top-right corner

### 2. System Status

```
┌────────┬────────┬────────┬────────┐
│Online  │ 3      │ 5      │ 2h     │
│Status  │Models  │Tools   │Uptime  │
└────────┴────────┴────────┴────────┘
┌──────────────────┬──────────────────┐
│ Models           │ Tools            │
│ - llama3.1:8b    │ - memory_search  │
│ - codestral:22b  │ - rag_search     │
│ - mistral:7b     │ - web_fetch      │
└──────────────────┴──────────────────┘
```

- 4-column stat grid (glass panels, centered values)
- 2-column grid below: Models list + Tools list
- Each model/tool in a subtle card with hover effect

### 3. Memory Browser

```
┌─────────────────────────────────────┐
│ [Search input]              142 mem │
├───────────┬───────────┬─────────────┤
│ Memory 1  │ Memory 2  │ Memory 3    │
│ glass     │ glass     │ glass       │
├───────────┼───────────┼─────────────┤
│ Memory 4  │ Memory 5  │ Memory 6    │
└───────────┴───────────┴─────────────┘
```

- Semantic search input at top
- 3-column grid of glass memory cards
- Each card: content text, tags (blue-tinted pills), meta timestamp, importance bar
- Hover: translateY(-2px) + brighter border

### 4. RAG Index

```
┌──────────────────┬──────────────────┐
│ Index Status     │ Document Search  │
│ 247 total        │ [search input]   │
│ 243 indexed      │ result 1         │
│ 4 failed         │ result 2         │
│ ████████████░░   │                  │
└──────────────────┴──────────────────┘
┌────────────────────────────────────┐
│ Indexed Files                      │
│ File | Chunks | Size | Status      │
│ ─────────────────────────────────  │
│ service.go | 12 | 8.4KB | indexed  │
└────────────────────────────────────┘
```

- Top: 2-column (status card with progress bar + search card)
- Bottom: full-width table of indexed files

## Interaction Patterns

- **Hover on cards:** border brightens to `rgba(255,255,255,0.18)`, subtle translateY
- **Active nav link:** `rgba(255,255,255,0.1)` background
- **Active conversation:** `rgba(255,255,255,0.08)` background
- **Focus on inputs:** border brightens to `rgba(255,255,255,0.22)`
- **Page transitions:** fadeIn animation (0.4s, translateY 6px)
- **Status dot:** pulsing glow animation (2.5s cycle)

## Technical Decisions

### Stack (unchanged)
- React 19 + TypeScript + Vite 6
- Tailwind CSS 4 (custom theme to match design tokens)
- TanStack React Query 5
- Lucide React icons
- React Router DOM 7

### Approach
- Replace existing Tailwind theme (colors, glass utilities)
- Rewrite layout: remove sidebar, add floating pill nav
- Rewrite all page components to new design
- Keep all API hooks and data fetching unchanged
- Keep all TypeScript types unchanged
- Conversations page removed — conversation list integrated into Chat sidebar

### Files to Modify
- `src/index.css` — new theme, glass utilities, animations
- `src/App.tsx` — new layout (remove sidebar, add pill nav)
- `src/components/layout/` — new NavPill component, remove old Sidebar
- `src/pages/Chat.tsx` — hybrid split layout
- `src/pages/SystemStatus.tsx` — new stat grid + cards
- `src/pages/MemoryBrowser.tsx` — glass card grid
- `src/pages/RAGStatus.tsx` — new layout
- `src/pages/ConversationHistory.tsx` — remove (merged into Chat)
- `src/components/chat/` — restyle messages, input, sidebar
- `src/components/memory/` — restyle to glass cards
- `src/components/rag/` — restyle
- `src/components/system/` — restyle
- `src/components/ui/` — update base components (Card, Button, Badge, Input, Tabs)

### Files NOT Modified
- `src/api/*` — all API clients stay the same
- `src/hooks/*` — all React Query hooks stay the same
- `src/lib/*` — utilities stay the same
- `dashboard/embed.go` — no changes
- `dashboard/vite.config.ts` — no changes
