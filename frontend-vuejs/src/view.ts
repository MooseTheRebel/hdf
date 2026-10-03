// View is the screen App shows. Views navigate by emitting the next View;
// this replaces the vanilla GUI's displayXView() functions (no router).
export type View =
    | {name: 'home'}
    | {name: 'diff'}
    | {name: 'status'}
    | {name: 'config'}
    | {name: 'daemonStatus'}
    | {name: 'daemonManagement'}
    | {name: 'init'}
    | {name: 'enroll'; path: string; pickError?: string}
    | {name: 'link'; noFetch: boolean}
    | {name: 'promote'}
    | {name: 'reportIssue'};
