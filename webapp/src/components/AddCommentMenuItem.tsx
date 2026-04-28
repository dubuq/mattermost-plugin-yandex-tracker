import { getStrings } from '../i18n';
import { openAddCommentModal } from './AddCommentModal';

// Comment bubble icon — outline style to match Mattermost's Compass icon set.
const CommentIcon = () => (
    <svg
        width='18'
        height='18'
        viewBox='0 0 24 24'
        fill='none'
        xmlns='http://www.w3.org/2000/svg'
        aria-hidden='true'
    >
        <path
            d='M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z'
            stroke='currentColor'
            strokeWidth='2'
            strokeLinecap='round'
            strokeLinejoin='round'
        />
    </svg>
);

export const AddCommentMenuItem = ({ postId }: { postId: string }) => {
    const t = getStrings();
    return (
        <button
            className='yt-plugin-menu-item'
            onClick={() => openAddCommentModal(postId)}
        >
            <span className='yt-plugin-menu-item__icon'>
                <CommentIcon />
            </span>
            {t.menuAction}
        </button>
    );
};
