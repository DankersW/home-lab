// Mobile shell screens + responsive root that picks mobile vs desktop.
(function () {
  const { useState, useMemo } = React;
  const RC = window.RC;
  const blank = RC.blank;

  // ---------- Add (home) ----------
  function AddScreen({ go, toast }) {
    const [rec, setRec] = useState(blank());
    function save() {
      if (!rec.title.trim()) return;
      Store.add(rec);
      setRec(blank());
      toast('Receipt saved');
      go({ name: 'list' });
    }
    return (
      <div className="screen fade-in">
        <div className="bar">
          <span className="mark"><Ico name="doc" className="ico"/></span>
          <div><h1>New receipt</h1><p className="sub">Add it now, find it later</p></div>
        </div>
        <div className="body">
          <ReceiptForm value={rec} onChange={setRec}/>
        </div>
        <div className="footer split">
          <button className="btn btn-ghost" onClick={() => go({ name: 'list' })}>
            <Ico name="search" className="ico"/> Find a receipt
          </button>
          <button className="btn btn-primary" disabled={!rec.title.trim()} onClick={save}>
            <Ico name="check" className="ico"/> Save
          </button>
        </div>
      </div>
    );
  }

  // ---------- List / Search ----------
  function ListScreen({ go, items }) {
    const [q, setQ] = useState('');
    const [tag, setTag] = useState(null);
    const tags = useMemo(() => Store.allTags(), [items]);

    const filtered = items.filter(it => {
      if (tag && !(it.tags || []).includes(tag)) return false;
      if (!q.trim()) return true;
      const hay = [it.title, it.merchant, it.notes, (it.tags || []).join(' ')].join(' ').toLowerCase();
      return q.toLowerCase().split(/\s+/).every(w => hay.includes(w));
    });

    return (
      <div className="screen fade-in">
        <div className="bar">
          <div><h1>Your receipts</h1><p className="sub">{items.length} item{items.length === 1 ? '' : 's'} on file</p></div>
          <button className="iconbtn" style={{ marginLeft: 'auto' }} onClick={() => go({ name: 'add' })} aria-label="Add receipt">
            <Ico name="plus" className="ico"/>
          </button>
        </div>

        <div className="searchbar">
          <Ico name="search" className="ico"/>
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search title, store, tag, notes…"/>
          {q && <button className="clear" onClick={() => setQ('')}><Ico name="x" className="ico"/></button>}
        </div>

        {tags.length > 0 && (
          <div className="filters">
            <button className={'fchip' + (!tag ? ' on' : '')} onClick={() => setTag(null)}>All</button>
            {tags.map(t => (
              <button key={t} className={'fchip' + (tag === t ? ' on' : '')} onClick={() => setTag(tag === t ? null : t)}>{t}</button>
            ))}
          </div>
        )}

        <div className="body">
          {filtered.length === 0 ? (
            <div className="empty">
              <Ico name="search" className="ic"/>
              <b>Nothing found</b>
              <p>{items.length === 0 ? 'Add your first receipt to start your archive.' : 'Try a different word or clear the filter.'}</p>
            </div>
          ) : (
            <React.Fragment>
              <div className="count">{filtered.length} result{filtered.length === 1 ? '' : 's'}</div>
              <div className="list">
                {filtered.map(it => <RC.CardRow key={it.id} it={it} onClick={() => go({ name: 'detail', id: it.id })}/>)}
              </div>
            </React.Fragment>
          )}
        </div>
      </div>
    );
  }

  // ---------- Detail ----------
  function DetailScreen({ go, id, askDelete }) {
    const it = Store.get(id);
    if (!it) { go({ name: 'list' }); return null; }
    return (
      <div className="screen fade-in">
        <div className="shead">
          <button className="iconbtn ghost" onClick={() => go({ name: 'list' })} aria-label="Back"><Ico name="back" className="ico"/></button>
          <div className="spacer"></div>
          <button className="iconbtn" onClick={() => go({ name: 'edit', id })} aria-label="Edit"><Ico name="pencil" className="ico"/></button>
          <button className="iconbtn danger" onClick={() => askDelete(it)} aria-label="Delete"><Ico name="trash" className="ico"/></button>
        </div>
        <div className="body">
          <RC.DetailBody it={it}/>
          <div style={{ height: 12 }}></div>
        </div>
      </div>
    );
  }

  // ---------- Edit ----------
  function EditScreen({ go, id, toast }) {
    const existing = Store.get(id);
    const [rec, setRec] = useState(existing || blank());
    if (!existing) { go({ name: 'list' }); return null; }
    function save() {
      if (!rec.title.trim()) return;
      Store.update(id, rec);
      toast('Changes saved');
      go({ name: 'detail', id });
    }
    return (
      <div className="screen fade-in">
        <div className="shead">
          <button className="iconbtn ghost" onClick={() => go({ name: 'detail', id })} aria-label="Cancel"><Ico name="back" className="ico"/></button>
          <h2>Edit receipt</h2>
        </div>
        <div className="body">
          <ReceiptForm value={rec} onChange={setRec}/>
        </div>
        <div className="footer split">
          <button className="btn btn-soft" onClick={() => go({ name: 'detail', id })}>Cancel</button>
          <button className="btn btn-primary" disabled={!rec.title.trim()} onClick={save}>
            <Ico name="check" className="ico"/> Save changes
          </button>
        </div>
      </div>
    );
  }

  // ---------- Mobile shell ----------
  function MobileShell() {
    const [screen, setScreen] = useState({ name: 'add' });
    const [items, setItems] = useState(Store.all());
    const [toastMsg, setToastMsg] = useState('');
    const [del, setDel] = useState(null);

    const refresh = () => setItems(Store.all());
    function go(s) { refresh(); setScreen(s); }
    function toast(m) { setToastMsg(m); clearTimeout(window.__t); window.__t = setTimeout(() => setToastMsg(''), 2200); }
    function confirmDelete() { Store.remove(del.id); setDel(null); toast('Receipt deleted'); go({ name: 'list' }); }

    let view;
    if (screen.name === 'add') view = <AddScreen go={go} toast={toast}/>;
    else if (screen.name === 'list') view = <ListScreen go={go} items={items}/>;
    else if (screen.name === 'detail') view = <DetailScreen go={go} id={screen.id} askDelete={setDel}/>;
    else if (screen.name === 'edit') view = <EditScreen go={go} id={screen.id} toast={toast}/>;

    return (
      <div className="scene">
        <div className="app">
          {view}
          <RC.Toast msg={toastMsg}/>
          {del && (
            <div className="sheet-bg" onClick={(e) => { if (e.target === e.currentTarget) setDel(null); }}>
              <div className="sheet">
                <h3>Delete this receipt?</h3>
                <p>“{del.title}” and its files will be removed. This can't be undone.</p>
                <div className="acts">
                  <button className="btn btn-danger" onClick={confirmDelete}><Ico name="trash" className="ico"/> Delete receipt</button>
                  <button className="btn btn-soft" onClick={() => setDel(null)}>Keep it</button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    );
  }

  // ---------- Responsive root ----------
  function Root() {
    const desktop = RC.useMedia('(min-width: 960px)');
    return desktop ? <DesktopShell/> : <MobileShell/>;
  }

  ReactDOM.createRoot(document.getElementById('root')).render(<Root/>);
})();
