// Shared form pieces: capture/upload zone, tag editor, the receipt form.
(function () {
  const { useState, useRef } = React;

  function Thumbs({ files, onRemove }) {
    if (!files.length) return null;
    return (
      <div className="thumbs">
        {files.map((f, i) => (
          <div className="thumb" key={i}>
            {f.type && f.type.startsWith('image') ? (
              <img src={f.dataUrl} alt=""/>
            ) : (
              <div className="pdf"><Ico name="doc" className="ico"/><small>PDF</small></div>
            )}
            <button className="rm" onClick={() => onRemove(i)} aria-label="Remove"><Ico name="x" className="ico"/></button>
          </div>
        ))}
      </div>
    );
  }

  function CaptureZone({ files, setFiles }) {
    const camRef = useRef(null);
    const fileRef = useRef(null);

    async function handle(e) {
      const picked = Array.from(e.target.files || []);
      const read = await Promise.all(picked.map(window.rcpReadFile));
      setFiles(files.concat(read));
      e.target.value = '';
    }

    return (
      <div className="capwrap">
        <div className="cap">
          <Ico name="camera" className="ic"/>
          <b>{files.length ? 'Add another file' : 'Snap a photo or add a PDF'}</b>
          <span>Receipt, warranty card, manual…</span>
          <div className="btns">
            <button onClick={() => camRef.current.click()}><Ico name="camera" className="ico"/> Take photo</button>
            <button onClick={() => fileRef.current.click()}><Ico name="upload" className="ico"/> Upload file</button>
          </div>
          <Thumbs files={files} onRemove={(i) => setFiles(files.filter((_, j) => j !== i))}/>
        </div>
        <input ref={camRef} type="file" accept="image/*" capture="environment" hidden onChange={handle}/>
        <input ref={fileRef} type="file" accept="image/*,application/pdf" multiple hidden onChange={handle}/>
      </div>
    );
  }

  function TagEditor({ tags, setTags }) {
    const [draft, setDraft] = useState('');
    function commit() {
      const t = draft.trim().replace(/,$/, '').toLowerCase();
      if (t && !tags.includes(t)) setTags(tags.concat(t));
      setDraft('');
    }
    function onKey(e) {
      if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); commit(); }
      else if (e.key === 'Backspace' && !draft && tags.length) setTags(tags.slice(0, -1));
    }
    return (
      <div className="fld">
        <label>Tags</label>
        <div className="tagbox" onClick={(e) => e.currentTarget.querySelector('input').focus()}>
          {tags.map((t, i) => (
            <span className="tagchip" key={i}>{t}
              <button onClick={() => setTags(tags.filter((_, j) => j !== i))}><Ico name="x" className="ico"/></button>
            </span>
          ))}
          <input value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={onKey} onBlur={commit}
            placeholder={tags.length ? 'Add tag…' : 'garden, tools, warranty…'}/>
        </div>
      </div>
    );
  }

  // The full receipt form (used by Add and Edit). Calls onChange(record) live.
  window.ReceiptForm = function ({ value, onChange }) {
    const v = value;
    const set = (patch) => onChange(Object.assign({}, v, patch));
    return (
      <React.Fragment>
        <CaptureZone files={v.files || []} setFiles={(files) => set({ files })}/>
        <div className="form">
          <div className="fld">
            <label>Title</label>
            <input className="in" value={v.title} placeholder="e.g. Robot lawnmower"
              onChange={(e) => set({ title: e.target.value })}/>
          </div>
          <div className="row">
            <div className="fld">
              <label>Merchant</label>
              <input className="in" value={v.merchant} placeholder="Bauhaus"
                onChange={(e) => set({ merchant: e.target.value })}/>
            </div>
            <div className="fld">
              <label>Amount</label>
              <div className="amount-wrap">
                <input className="in" value={v.amount} placeholder="0" inputMode="decimal"
                  onChange={(e) => set({ amount: e.target.value })}/>
                <span className="cur">kr</span>
              </div>
            </div>
          </div>
          <div className="fld">
            <label>Date of purchase</label>
            <input className="in" type="date" value={v.date}
              onChange={(e) => set({ date: e.target.value })}/>
          </div>
          <TagEditor tags={v.tags || []} setTags={(tags) => set({ tags })}/>
          <div className="fld">
            <label>Notes &amp; context</label>
            <textarea value={v.notes} placeholder="Serial number, where it's installed, warranty length, anything useful later…"
              onChange={(e) => set({ notes: e.target.value })}></textarea>
          </div>
        </div>
      </React.Fragment>
    );
  };
})();
