import "./styles.css";

export default function App() {
  return (
    <main className="app-shell">
      <section className="panel">
        <p className="eyebrow">Transcodex Demo</p>
        <h1>Upload a video and track the processing job.</h1>
        <form className="upload-form">
          <label>
            Video file
            <input type="file" accept="video/*" />
          </label>
          <label>
            Priority
            <select defaultValue="0">
              <option value="0">Normal</option>
              <option value="1">High</option>
              <option value="2">Urgent</option>
            </select>
          </label>
          <button type="button">Upload</button>
        </form>
      </section>
    </main>
  );
}
