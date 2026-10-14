# 14: Phone drawer list with one-column cards

**What to build:** Phones follow the same shell as desktop, as in the mock: below `md` the Rail and second sidebar are hidden, and the Sheet drawer (`w-80`, `max-w-[88vw]`) carries the same Quota list as the desktop second sidebar in one scroll region. Cards stack in one column and every control is an easy hit.

**Blocked by:** 13-quota-sidebar

**Status:** done

- [x] The phone drawer shows the same Live status list as the desktop second sidebar
- [x] Cards lay out in one column on narrow screens
- [x] All buttons and tabs meet the 44px minimum target (`max-md:h-11`); inputs are 16px
- [x] Vitest covers the drawer list and layout behaviour; repo check and `npm test` pass
