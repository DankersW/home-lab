// Desktop web shell — sidebar + searchable list + detail pane, with a form modal.
(function () {
  const { useState, useMemo } = React;
  const RC = window.RC;

  // Modal hosting the receipt form (Add or Edit).
  function FormModal({ mode, id, onClose, onSaved }) {
    const init = mode === 'edit' ? Store.get(id) : RC.blank();
    const [rec, setRec] = useState(init || RC.blank());
    function save() {
      if (!rec.title.trim()) return;
      const saved = mode === 'edit' ? Store.update(id, rec) : Store.add(rec);
      onSaved(saved, mode);
    }
    return (
      <div className="modal-bg" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
        <div className="modal">
          <div className="mhead">
            <h2>{mode === 'edit' ? 'Edit receipt' : 'New receipt'}</h2>
            <button className="iconbtn ghost" onClick={onClose} aria-label="Close"><Ico name="x" className="ico"/></button>
          </div>
          <div className="mbody">
            <ReceiptForm value={rec} onChange={setRec}/>
          </div>
          <div className="footer split">
            <button className="btn btn-soft" onClick={onClose}>Cancel</button>
            <button className="btn btn-primary" disabled={!rec.title.trim()} onClick={save}>
              <Ico name="check" className="ico"/> {mode === 'edit' ? 'Save changes' : 'Save receipt'}
            </button>
          </div>
        </div>
      </div>
    );
  }

  window.DesktopShell = function DesktopShell() {
    const [items, setItems] = useState(Store.all());
    const [q, setQ] = useState('');
    const [tag, setTag] = useState(null);
    const [selId, setSelId] = useState(null);
    const [modal, setModal] = useState(null);   // {mode:'add'|'edit', id?}
    const [del, setDel] = useState(null);
    const [toastMsg, setToastMsg] = useState('');

    const refresh = () => setItems(Store.all());
    function toast(m) { setToastMsg(m); clearTimeout(window.__t); window.__t = setTimeout(() => setToastMsg(''), 2200); }

    const tagCounts = useMemo(() => {
      const c = {};
      items.forEach(it => (it.tags || []).forEach(t => { c[t] = (c[t] || 0) + 1; }));
      return c;
    }, [items]);
    const tags = useMemo(() => Object.keys(tagCounts).sort((a, b) => tagCounts[b] - tagCounts[a]), [tagCounts]);

    const filtered = items.filter(it => {
      if (tag && !(it.tags || []).includes(tag)) return false;
      if (!q.trim()) return true;
      const hay = [it.title, it.merchant, it.notes, (it.tags || []).join(' ')].join(' ').toLowerCase();
      return q.toLowerCase().split(/\s+/).every(w => hay.includes(w));
    });

    const sel = selId ? Store.get(selId) : null;

    function onSaved(saved, mode) {
      setModal(null); refresh();
      if (saved) setSelId(saved.id);
      toast(mode === 'edit' ? 'Changes saved' : 'Receipt saved');
    }
    function confirmDelete() {
      const wasSel = del.id === selId;
      Store.remove(del.id); setDel(null); refresh();
      if (wasSel) setSelId(null);
      toast('Receipt deleted');
    }

    return (
      <div className="web">
        {/* Sidebar */}
        <aside className="dnav">
          <div className="dbrand">
            <span className="mark"><Ico name="doc" className="ico"/></span>
            <div><h1>Receipts</h1><div className="sub">Household archive</div></div>
          </div>
          <button className="btn btn-primary" onClick={() => setModal({ mode: 'add' })}>
            <Ico name="plus" className="ico"/> New receipt
          </button>
          <div className="dfilters">
            <div className="lbl">Filter by tag</div>
            <button className={'dfilt' + (!tag ? ' on' : '')} onClick={() => setTag(null)}>
              <span>All receipts</span><span className="n">{items.length}</span>
            </button>
            {tags.map(t => (
              <button key={t} className={'dfilt' + (tag === t ? ' on' : '')} onClick={() => setTag(tag === t ? null : t)}>
                <span>{t}</span><span className="n">{tagCounts[t]}</span>
              </button>
            ))}
          </div>
          <div className="foot">Stored on this device · {items.length} item{items.length === 1 ? '' : 's'}</div>
        </aside>

        {/* List column */}
        <section className="dlist">
          <div className="top">
            <div className="searchbar">
              <Ico name="search" className="ico"/>
              <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search title, store, tag, notes…"/>
              {q && <button className="clear" onClick={() => setQ('')}><Ico name="x" className="ico"/></button>}
            </div>
          </div>
          <div className="dlistscroll">
            {filtered.length === 0 ? (
              <div className="empty">
                <Ico name="search" className="ic"/>
                <b>Nothing found</b>
                <p>{items.length === 0 ? 'Add your first receipt to start your archive.' : 'Try a different word or clear the filter.'}</p>
              </div>
            ) : (
              <React.Fragment>
                <div className="count">{filtered.length} result{filtered.length === 1 ? '' : 's'}{tag ? ' · ' + tag : ''}</div>
                <div className="list">
                  {filtered.map(it => <RC.CardRow key={it.id} it={it} active={it.id === selId} onClick={() => setSelId(it.id)}/>)}
                </div>
              </React.Fragment>
            )}
          </div>
        </section>

        {/* Detail pane */}
        <main className="ddetail">
          {!sel ? (
            <div className="dempty">
              <Ico name="doc" className="ic"/>
              <b>Select a receipt</b>
              <p>Pick an item from the list to see its files, details and notes — or add a new one.</p>
            </div>
          ) : (
            <React.Fragment>
              <div className="dtoolbar">
                <button className="tbtn accent" onClick={() => setModal({ mode: 'edit', id: sel.id })}><Ico name="pencil" className="ico"/> Edit</button>
                <button className="tbtn danger" onClick={() => setDel(sel)}><Ico name="trash" className="ico"/> Delete</button>
              </div>
              <div className="dwrap fade-in" key={sel.id}>
                <RC.DetailBody it={sel}/>
              </div>
            </React.Fragment>
          )}
        </main>

        <RC.Toast msg={toastMsg}/>

        {modal && <FormModal mode={modal.mode} id={modal.id} onClose={() => setModal(null)} onSaved={onSaved}/>}

        {del && (
          <div className="modal-bg" onClick={(e) => { if (e.target === e.currentTarget) setDel(null); }}>
            <div className="modal confirm">
              <div className="mbody" style={{ padding: '24px 24px 8px' }}>
                <h3 style={{ fontFamily: 'var(--fh)', fontSize: 19, margin: '0 0 6px', fontWeight: 800 }}>Delete this receipt?</h3>
                <p style={{ fontSize: 13.5, color: 'var(--muted)', margin: 0, lineHeight: 1.5 }}>“{del.title}” and its files will be removed. This can't be undone.</p>
              </div>
              <div className="footer split">
                <button className="btn btn-soft" onClick={() => setDel(null)}>Keep it</button>
                <button className="btn btn-danger" onClick={confirmDelete}><Ico name="trash" className="ico"/> Delete</button>
              </div>
            </div>
          </div>
        )}
      </div>
    );
  };
})();
