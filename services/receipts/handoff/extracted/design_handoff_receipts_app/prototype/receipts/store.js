// Receipts data store — localStorage-backed CRUD with seed data.
(function () {
  const KEY = 'rcp_items_v1';

  const SEED = [
    { title: 'LG OLED C3 65"', merchant: 'Elgiganten', amount: '18990', date: '2024-11-02',
      tags: ['tv', 'living room'], notes: 'Wall-mounted. Serial on back panel. 5-yr extended warranty bought.', files: [] },
    { title: 'Synology DS923+', merchant: 'Inet', amount: '6499', date: '2025-01-20',
      tags: ['homelab', 'nas'], notes: '4-bay. Running 2× 8TB Iron Wolf. Receipt PDF + invoice.', files: [] },
    { title: 'Sonos Beam Gen 2', merchant: 'Webhallen', amount: '4490', date: '2024-03-15',
      tags: ['audio', 'living room'], notes: 'Paired with the TV via eARC.', files: [] },
    { title: 'Bosch Series 6 Dishwasher', merchant: 'Elon', amount: '8990', date: '2024-06-10',
      tags: ['kitchen', 'appliance'], notes: 'Installed by store. 5-year warranty card in the drawer.', files: [] },
    { title: 'Husqvarna Automower 305', merchant: 'Bauhaus', amount: '12490', date: '2025-04-22',
      tags: ['garden'], notes: 'Boundary wire installed along the back fence.', files: [] },
  ];

  function uid() { return 'r' + Date.now().toString(36) + Math.random().toString(36).slice(2, 7); }

  function load() {
    try {
      const raw = localStorage.getItem(KEY);
      if (raw) return JSON.parse(raw);
    } catch (e) {}
    const seeded = SEED.map((s, i) => Object.assign({ id: uid(), created: Date.now() - (5 - i) * 86400000 }, s));
    localStorage.setItem(KEY, JSON.stringify(seeded));
    return seeded;
  }

  function persist(items) { localStorage.setItem(KEY, JSON.stringify(items)); }

  window.Store = {
    all() { return load().sort((a, b) => (b.created || 0) - (a.created || 0)); },
    get(id) { return load().find(x => x.id === id) || null; },
    add(obj) {
      const items = load();
      const rec = Object.assign({ id: uid(), created: Date.now() }, obj);
      items.push(rec); persist(items); return rec;
    },
    update(id, obj) {
      const items = load();
      const i = items.findIndex(x => x.id === id);
      if (i >= 0) { items[i] = Object.assign({}, items[i], obj); persist(items); return items[i]; }
      return null;
    },
    remove(id) { persist(load().filter(x => x.id !== id)); },
    allTags() {
      const set = {};
      load().forEach(it => (it.tags || []).forEach(t => { set[t] = (set[t] || 0) + 1; }));
      return Object.keys(set).sort((a, b) => set[b] - set[a]);
    },
  };

  // Helpers
  window.rcpFmt = function (n) {
    const v = parseFloat(String(n).replace(/[^\d.]/g, ''));
    if (isNaN(v)) return '—';
    return v.toLocaleString('sv-SE');
  };
  window.rcpDate = function (iso) {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d)) return iso;
    return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' });
  };
  window.rcpReadFile = function (file) {
    return new Promise((res) => {
      const r = new FileReader();
      r.onload = () => res({ name: file.name, type: file.type, dataUrl: r.result });
      r.readAsDataURL(file);
    });
  };
})();
