// Shared building blocks used by BOTH the mobile and desktop shells.
(function () {
  const { useState, useEffect } = React;
  const RC = (window.RC = window.RC || {});

  RC.blank = () => ({ title: '', merchant: '', amount: '', date: new Date().toISOString().slice(0, 10), tags: [], notes: '', files: [] });

  RC.useMedia = function (q) {
    const [m, setM] = useState(() => window.matchMedia(q).matches);
    useEffect(() => {
      const mq = window.matchMedia(q);
      const h = (e) => setM(e.matches);
      mq.addEventListener('change', h);
      setM(mq.matches);
      return () => mq.removeEventListener('change', h);
    }, [q]);
    return m;
  };

  RC.Toast = function ({ msg }) {
    return <div className={'toast' + (msg ? ' show' : '')}><Ico name="check" className="ico"/> {msg || ''}</div>;
  };

  RC.thumbFor = function (it) {
    const img = (it.files || []).find(f => f.type && f.type.startsWith('image'));
    if (img) return <img src={img.dataUrl} alt=""/>;
    if ((it.files || []).length) return <Ico name="doc" className="ico"/>;
    return (it.title || '?').trim().charAt(0).toUpperCase() || '?';
  };

  // One list row, shared. `active` highlights it (desktop selection).
  RC.CardRow = function ({ it, onClick, active }) {
    return (
      <div className={'card' + (active ? ' active' : '')} onClick={onClick}>
        <div className="tmb">{RC.thumbFor(it)}</div>
        <div className="meta">
          <div className="nm">{it.title}</div>
          <div className="mc">{it.merchant || 'No merchant'} · {window.rcpDate(it.date)}</div>
          {(it.files || []).length > 0 && (
            <span className="badge"><Ico name="doc" className="ico"/> {it.files.length} file{it.files.length === 1 ? '' : 's'}</span>
          )}
        </div>
        <div className="amt"><b>{window.rcpFmt(it.amount)}</b><small>SEK</small></div>
      </div>
    );
  };

  // The detail content (gallery + hero + facts + notes), no toolbar — wrapped differently per shell.
  RC.DetailBody = function ({ it }) {
    const files = it.files || [];
    return (
      <React.Fragment>
        <div className="gallery">
          {files.length === 0 ? (
            <div className="g none"><Ico name="image" className="ico"/><span style={{ fontSize: 12 }}>No file attached</span></div>
          ) : files.map((f, i) => (
            <div className="g" key={i}>
              {f.type && f.type.startsWith('image')
                ? <img src={f.dataUrl} alt=""/>
                : <div className="pdf"><Ico name="doc" className="ico"/><a href={f.dataUrl} target="_blank" rel="noreferrer" download={f.name}>Open PDF</a></div>}
            </div>
          ))}
        </div>

        <div className="dhero">
          <div className="t">{it.title}</div>
          <div className="price">{window.rcpFmt(it.amount)} <small>kr</small></div>
        </div>

        <div className="facts">
          <div className="fact"><Ico name="shop" className="ico"/><span className="k">Merchant</span><span className="v">{it.merchant || '—'}</span></div>
          <div className="fact"><Ico name="cal" className="ico"/><span className="k">Purchased</span><span className="v">{window.rcpDate(it.date) || '—'}</span></div>
          {(it.tags || []).length > 0 && (
            <div className="fact" style={{ alignItems: 'flex-start' }}>
              <Ico name="tag" className="ico"/><span className="k">Tags</span>
              <span className="v dtags">{it.tags.map((t, i) => <span className="t" key={i}>{t}</span>)}</span>
            </div>
          )}
        </div>

        {it.notes && (
          <div className="notes">
            <div className="lbl">Notes &amp; context</div>
            <p>{it.notes}</p>
          </div>
        )}
      </React.Fragment>
    );
  };
})();
