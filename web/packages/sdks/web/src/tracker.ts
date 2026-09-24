import { OpenAnalyticsWeb } from './index';

((window: any) => {
  if (window.oa) {
    const queue = window.oa.q || [];
    const firstInit = queue.shift();
    const oa = new OpenAnalyticsWeb(firstInit ? firstInit[1] : {});
    queue.forEach((item: any[]) => {
      if (item[0] in oa) {
        (oa as any)[item[0]](...item.slice(1));
      }
    });

    const oaCallable = new Proxy(
      ((method: string, ...args: any[]) => {
        const fn = (oa as any)[method]
          ? (oa as any)[method].bind(oa)
          : undefined;
        if (typeof fn === 'function') {
          fn(...args);
        } else {
          console.warn(`[OpenAnalytics] ${method} is not a function`);
        }
      }) as typeof oa & ((method: string, ...args: any[]) => void),
      {
        get(_target, prop) {
          if (prop === 'q') return undefined;
          const value = (oa as any)[prop];
          if (typeof value === 'function') {
            return value.bind(oa);
          }
          return value;
        },
      },
    );

    window.oa = oaCallable;
    window.openanalytics = oa;
  }
})(window);
