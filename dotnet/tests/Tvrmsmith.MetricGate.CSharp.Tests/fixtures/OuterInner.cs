namespace Fixtures;

public class Outer
{
    public class Inner
    {
        public int M(bool b)
        {
            if (b)
            {
                return 1;
            }

            return 0;
        }
    }
}
