/* Metadata only: all page content always comes from the universal extractor. */
(() => {
  'use strict';
  globalThis.PageVaultExtractor.registerAdapter({
    id: 'gitlab',
    matches({ document, url }) {
      const host = new URL(url).hostname;
      return (
        /(^|\.)gitlab\.com$/i.test(host) ||
        !!document.querySelector(
          'meta[name="application-name"][content="GitLab" i], meta[name="gitlab-version"], meta[name="gitlab:version"], body[data-page][data-project-id]',
        )
      );
    },
    extract({ document, url }) {
      const meta = (name) => document.querySelector(`meta[name="${name}"]`)?.content || '';
      const body = document.body;
      const result = { platform: 'gitlab' };
      const projectId = meta('project_id') || body?.dataset.projectId;
      const namespaceId = meta('namespace_id') || body?.dataset.namespaceId;
      if (projectId) result.project_id = String(projectId);
      if (namespaceId) result.namespace_id = String(namespaceId);
      if (body?.dataset.page) result.page = body.dataset.page;
      const match = new URL(url).pathname.match(
        /^\/(.+?)\/-\/(issues|merge_requests)\/(\d+)(?:\/|$)/,
      );
      if (match) {
        result.project_path = match[1];
        result.entity_type = match[2] === 'issues' ? 'issue' : 'merge_request';
        result.iid = match[3];
      }
      return result;
    },
  });
})();
